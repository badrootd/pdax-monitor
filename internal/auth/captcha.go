package auth

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

const (
	okPrefix                  = "OK|"
	zeroBalance               = "ERROR_ZERO_BALANCE"
	notReadyStatus            = "CAPCHA_NOT_READY"
	solverPostTaskURL         = "https://2captcha.com/in.php?key=%s&method=userrecaptcha&googlekey=%s&pageurl=%s"
	solverGetResultURL        = "https://2captcha.com/res.php?key=%s&action=get&googlekey=%s&id=%s"
	solutionCheckPeriodSec    = 5
	solutionCheckAttemptCount = 30 // in practice captcha is solved less than a minute
)

// CaptchaSolver is service to solve captcha.
type CaptchaSolver struct {
	SolverKey   string
	TaskKey     string
	TaskPageURL string
	Logger      log.Logger
}

// Solve is used to solve login form captchas.
func (cs *CaptchaSolver) Solve() (string, error) {
	resp, err := http.Get(fmt.Sprintf(solverPostTaskURL, cs.SolverKey, cs.TaskKey, cs.TaskPageURL))
	if err != nil {
		return "", fmt.Errorf("failed to post captcha task to 2captcha.com: %v", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	task := string(body)

	if task == zeroBalance {
		return "", fmt.Errorf("2captcha zero balance")
	}

	if strings.HasPrefix(task, okPrefix) {
		taskID := strings.TrimPrefix(task, okPrefix)

		level.Info(cs.Logger).Log("msg", "captcha task scheduled", "url", cs.TaskPageURL)
		for i := 0; i < solutionCheckAttemptCount; i++ {
			// Wait till captcha solved
			time.Sleep(solutionCheckPeriodSec * time.Second)

			resp, err = http.Get(fmt.Sprintf(solverGetResultURL, cs.SolverKey, cs.TaskKey, taskID))
			if err != nil {
				return "", fmt.Errorf("failed to get captcha result from 2captcha.com: %v", err)
			}
			defer resp.Body.Close()

			body, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				return "", err
			}

			bodyString := string(body)
			if bodyString == notReadyStatus {
				level.Info(cs.Logger).Log("msg", "captcha solution not ready yet", "awaitSeconds", solutionCheckPeriodSec)
				continue
			}

			if strings.HasPrefix(bodyString, okPrefix) {
				level.Info(cs.Logger).Log("msg", "captcha solved", "solution", strings.TrimPrefix(bodyString, okPrefix))
				return strings.TrimPrefix(bodyString, okPrefix), nil
			}
		}

		return "", fmt.Errorf("captcha solution await timeout")
	}

	return "", nil
}
