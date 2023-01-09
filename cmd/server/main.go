package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"net/http"

	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	kitlevel "github.com/go-kit/log/level"
	"github.com/oklog/run"
	"github.com/peterbourgon/ff"
	"github.com/peterbourgon/ff/ffyaml"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/pudgydoge/pdax-monitor/internal/auth"
	"github.com/pudgydoge/pdax-monitor/internal/pg"
	"github.com/pudgydoge/pdax-monitor/internal/rollbar"
	"github.com/pudgydoge/pdax-monitor/internal/service"
	"github.com/pudgydoge/pdax-monitor/internal/websocket"
	"github.com/spf13/viper"
)

// exitCode is a process termination code.
type exitCode int

// Possible process termination codes are listed below.
const (
	// exitSuccess is code for successful program termination.
	exitSuccess exitCode = 0
	// exitFailure is code for unsuccessful program termination.
	exitFailure exitCode = 1
)

// version is the service version from git tag.
var version = ""

const (
	defaultPDAXAuthURL         = "https://trade.pdax.ph/moon/v1/login"
	defaultPDAXSignInURL       = "https://trade.pdax.ph/signin"
	defaultPDAXAuthRefreshURL  = "https://trade.pdax.ph/moon/v1/refreshToken"
	defaultPDAXTradeURL        = "wss://trade.pdax.ph/tradeui/ws/master"
	defaultCaptchaRecaptchaKey = "6Lcj_WQUAAAAAH7U8sEordiEHPEJDdVzoKQiH7Oa"
	defaultSolverServiceKey    = "captcha-solver-key"

	LevelError = "error"
	LevelWarn  = "warn"
	LevelInfo  = "info"
	LevelDebug = "debug"
)

func main() {
	os.Exit(int(gracefulMain()))
}

// gracefulMain releases resources gracefully upon termination.
// When we call os.Exit defer statements do not run resulting in unclean process shutdown.
func gracefulMain() exitCode {
	var err error
	logger, logConfigHandler := WithConfigHandler("debug")
	http.DefaultServeMux.HandleFunc("/logging", logConfigHandler)

	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	// Kubernetes (rolling update) doesn't wait until a pod is out of rotation before sending SIGTERM,
	// and external LB could still route traffic to a non-existing pod resulting in a surge of 50x API errors.
	// It's recommended to wait for 5 seconds before terminating the program; see references
	// https://github.com/kubernetes-retired/contrib/issues/1140, https://youtu.be/me5iyiheOC8?t=1797.
	shutdownDelay := fs.Duration("shutdown-delay", 5*time.Second, "Delay before application shutdown")
	pgString := fs.String(
		"pg.conn-string",
		"user=postgres password=postgres host=127.0.0.1 port=5432 dbname=pdax_test connect_timeout=3 sslmode=disable",
		`Postgres connection string`,
	)
	pdaxUsername := fs.String("pdax.username", "", "PDAX username")
	pdaxPassword := fs.String("pdax.password", "", "PDAX password")
	pdaxAuthURL := fs.String("pdax.auth-url", defaultPDAXAuthURL,
		"PDAX backend URL to which page refers after filling user,pass and solved gcaptcha")
	pdaxAuthRefreshURL := fs.String("pdax.auth-refresh-url", defaultPDAXAuthRefreshURL, "PDAX backend URL to query for JWT token")
	pdaxTradeURL := fs.String("pdax.trade-url", defaultPDAXTradeURL, "PDAX trading page main websocket")
	captchaSolverKey := fs.String("captcha.solver-key", defaultSolverServiceKey, "Recaptcha solver access key")
	captachTaskURL := fs.String("captcha.task-url", defaultPDAXSignInURL, "PDAX page URL with login form and captcha. Used by captcha solver")
	captchaTask := fs.String("captcha.task", defaultCaptchaRecaptchaKey, "Recaptcha key generated by PDAX itself")
	opsHTTPAddr := fs.String("ops.http-addr", ":8081", "HTTP ops API address to listen")
	fs.String("config", "", "config file (optional)")
	bookPath := fs.String("wsInitBook", "./auxiliary/wsbook.json", "Path to the PDAX websocket connection bootstrap book")
	currencyCodesPath := fs.String("currencyCodes", "./auxiliary/currencyCodes.json", "Path to the PDAX currency codes file")
	rollbarEnv := fs.String("rollbar.env", "development", "Rollbar environment")
	rollbarToken := fs.String("rollbar.token", "", "Rollbar token")
	rollbarIsActive := fs.Bool("rollbar.is_active", false, "Rollbar enabled")
	v := fs.Bool("v", false, "Show version")

	fmt.Println(pdaxUsername)
	fmt.Println(pdaxPassword)
	fmt.Println(captchaSolverKey)

	if err = ff.Parse(
		fs, os.Args[1:],
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(ffyaml.Parser),
	); err != nil {
		fmt.Println(err)
		return exitFailure
	}

	if err != nil {
		level.Error(logger).Log("msg", "parsing cli flags failed", "err", err)
		return exitFailure
	}

	if *rollbarIsActive {
		rollbar.SetUp(logger, *rollbarToken, *rollbarEnv)
		defer rollbar.TearDown()
	}

	if *v {
		if version == "" {
			fmt.Println("Version not set")
		} else {
			fmt.Printf("Version: %s\n", version)
		}

		return exitSuccess
	}

	wsInitBook, err := unmarshalWsInitBook(*bookPath)
	if err != nil {
		level.Error(logger).Log("msg", "parsing wsInitBook failed", "err", err)
		return exitFailure
	}

	currencyCodes, err := unmarshalCurrencyCodes(*currencyCodesPath)
	if err != nil {
		level.Error(logger).Log("msg", "parsing currencyCodes failed", "err", err)
		return exitFailure
	}

	// It's nice to be able to see panics in Rollbar, hence we monitor for panics after
	// logger has been bootstrapped with Rollbar.
	defer monitorPanic(logger)

	// Expose endpoint for healthcheck.
	http.DefaultServeMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	// Expose the registered Prometheus metrics via HTTP.
	http.DefaultServeMux.Handle("/metrics", promhttp.Handler())

	opsServer := http.Server{
		Addr:    *opsHTTPAddr,
		Handler: http.DefaultServeMux,
	}

	var pgClient *pg.Client
	{
		pgClient = pg.NewClient(
			pg.WithLogger(logger),
		)
		if err := pgClient.Open(*pgString); err != nil {
			level.Error(logger).Log("msg", "db connection failed", "err", err)
			return exitFailure
		}

		defer func() {
			if err := pgClient.Close(); err != nil {
				level.Warn(logger).Log("msg", "db close failed", "err", err)
			}
		}()
	}

	captchaSolver := auth.CaptchaSolver{
		SolverKey:   *captchaSolverKey,
		TaskKey:     *captchaTask,
		TaskPageURL: *captachTaskURL,
		Logger:      logger,
	}

	authService := auth.NewAuthService(
		*pdaxAuthURL,
		*pdaxAuthRefreshURL,
		auth.WithCaptchaSolver(captchaSolver),
		auth.WithLogger(logger),
		auth.WithCredentials(*pdaxUsername, *pdaxPassword),
	)

	tradeMonitor := service.NewMonitorService(
		service.WithAuthService(authService),
		service.WithTradeURL(*pdaxTradeURL),
		service.WithTradeRepository(pgClient.TradeRepository()),
		service.WithOrderRepository(pgClient.OrderRepository()),
		service.WithCurrencyCodes(currencyCodes),
		service.WithLogger(logger),
	)

	ctx, cancel := context.WithCancel(context.Background())
	var g run.Group
	{
		g.Add(func() error {
			logger.Log("msg", "trade monitoring is starting")

			err = tradeMonitor.MonitorWithRecovery(ctx, wsInitBook)
			if err != nil {
				level.Error(logger).Log("msg", "monitor trades error", "err", err)
			}

			logger.Log("msg", "trade monitoring stopped")
			return nil
		}, func(err error) {
			logger.Log("msg", "trade monitoring was interrupted", "err", err)
			cancel()
		})
	}
	{
		g.Add(func() error {
			// It's nice to be able to see panics in Rollbar.
			defer monitorPanic(logger)

			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
			select {
			case <-ctx.Done():
				return nil
			case s := <-sig:
				message := fmt.Sprintf("signal received (waiting %v before terminating): %v", *shutdownDelay, s)
				level.Info(logger).Log("msg", message)
				time.Sleep(*shutdownDelay)
				level.Info(logger).Log("msg", "terminating...")
				return nil
			}
		}, func(_ error) {
			level.Info(logger).Log("msg", "program was interrupted")
			cancel()
		})
	}
	{
		g.Add(func() error {
			// It's nice to be able to see panics in Rollbar.
			defer monitorPanic(logger)

			level.Info(logger).Log("msg", "ops server is starting", "addr", opsServer.Addr)
			return opsServer.ListenAndServe()
		}, func(_ error) {
			level.Info(logger).Log("msg", "ops server was interrupted")

			defer cancel()

			shErr := opsServer.Shutdown(ctx)

			if shErr != nil {
				level.Error(logger).Log("msg", "ops server shut down with error", "shErr", shErr)
			}
		})
	}

	err = g.Run()
	if err != nil {
		level.Error(logger).Log("msg", "actors stopped gracefully", "err", err)
		return exitFailure
	}

	return exitSuccess
}

// monitorPanic monitors panics and reports them somewhere (e.g. logs, Rollbar, ...).
func monitorPanic(logger log.Logger) {
	if rec := recover(); rec != nil {
		err := fmt.Sprintf("panic: %v \n stack trace: %s", rec, debug.Stack())
		level.Error(logger).Log("err", err)
		panic(err)
	}
}

func unmarshalCurrencyCodes(currencyCodesPath string) (map[int]string, error) {
	currencyCodes := make(map[int]string)
	if _, err := os.Stat(currencyCodesPath); !os.IsNotExist(err) {
		viper.SetConfigFile(currencyCodesPath)
		err = viper.ReadInConfig()
		if err != nil {
			err = fmt.Errorf("failed to read currency codes file: %v", err)
			return map[int]string{}, err
		}

		for key, value := range viper.GetStringMapString("currencyCodes") {
			keyInt, _ := strconv.Atoi(key)
			currencyCodes[keyInt] = value
		}
	}

	return currencyCodes, nil
}

func unmarshalWsInitBook(bookPath string) (websocket.InitBook, error) {
	var wsInitBook websocket.InitBook
	if _, err := os.Stat(bookPath); !os.IsNotExist(err) {
		viper.SetConfigFile(bookPath)
		err = viper.ReadInConfig()
		if err != nil {
			err = fmt.Errorf("failed to read binary book: %v", err)
			return websocket.InitBook{}, err
		}

		err = viper.Unmarshal(&wsInitBook)
		if err != nil {
			err = fmt.Errorf("unmarshaling binary book failed: %v", err)
			return websocket.InitBook{}, err
		}
	}

	return wsInitBook, nil
}

func WithConfigHandler(level string) (*log.SwapLogger, http.HandlerFunc) {
	var baseLogger log.Logger
	{
		baseLogger = log.NewJSONLogger(log.NewSyncWriter(os.Stderr))
		baseLogger = log.With(baseLogger, "ts", log.DefaultTimestampUTC)
		baseLogger = log.With(baseLogger, "caller", log.DefaultCaller)
	}

	var standardLogger log.Logger
	switch level {
	case LevelError:
		standardLogger = kitlevel.NewFilter(baseLogger, kitlevel.AllowError())
	case LevelWarn:
		standardLogger = kitlevel.NewFilter(baseLogger, kitlevel.AllowWarn())
	case LevelInfo:
		standardLogger = kitlevel.NewFilter(baseLogger, kitlevel.AllowInfo())
	default:
		standardLogger = baseLogger
	}

	logger := log.SwapLogger{}
	logger.Swap(standardLogger)

	f := func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("form parse failed"))
			return
		}

		switch r.Form.Get("level") {
		case LevelError:
			logger.Swap(kitlevel.NewFilter(baseLogger, kitlevel.AllowError()))
			w.Write([]byte("log level is set to error"))
		case LevelWarn:
			logger.Swap(kitlevel.NewFilter(baseLogger, kitlevel.AllowWarn()))
			w.Write([]byte("log level is set to warn"))
		case LevelInfo:
			logger.Swap(kitlevel.NewFilter(baseLogger, kitlevel.AllowInfo()))
			w.Write([]byte("log level is set to info"))
		case LevelDebug:
			k, v := r.Form.Get("key"), r.Form.Get("value")
			if k == "" || v == "" {
				logger.Swap(baseLogger)
				w.Write([]byte("log level is set to debug"))
				return
			}

			logger.Swap(&FilterLogger{
				Hit:   baseLogger,
				Miss:  standardLogger,
				Key:   k,
				Value: v,
			})
			fmt.Fprintf(w, "log level is set to filtered debug: %s=%s", k, v)
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("unknown log level, try error, warn, info, debug"))
		}
	}

	return &logger, f
}

type FilterLogger struct {
	Hit   log.Logger
	Miss  log.Logger
	Key   string
	Value string
}

func (l *FilterLogger) Log(keyvals ...interface{}) error {
	for i := 0; i < len(keyvals); i += 2 {
		if k := fmt.Sprint(keyvals[i]); k == l.Key {
			if v := fmt.Sprint(keyvals[i+1]); v == l.Value {
				return l.Hit.Log(keyvals...)
			}
		}
	}
	return l.Miss.Log(keyvals...)
}
