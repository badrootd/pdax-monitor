MIN_COVERAGE = 70
test:
	go test ./... -v -race -count=1 -cover -coverprofile=coverage.txt && go tool cover -func=coverage.txt \
	| grep total | tee /dev/stderr | sed 's/\%//g' | awk '{err=0;c+=$$3}{if (c > 0 && c < $(MIN_COVERAGE)) {printf "=== FAIL: Coverage failed at %.2f%%\n", c; err=1}} END {exit err}'

format:
	goimports -local "github.com/pudgydoge/pdax-monitor" -w ./
	# We need to run `gofmt` with `-s` flag as well (best practice, linters require it).
	# `goimports` doesn't support `-s` flag just yet.
	# For details see https://github.com/golang/go/issues/21476
	gofmt -w -s ./

lint:
	golangci-lint run --deadline=5m -v

gosec:
	gosec -exclude=G104 -fmt=json -exclude-dir=.go ./...

lint_docker:
	docker run --rm -v $(GOPATH)/pkg/mod:/go/pkg/mod:ro -v `pwd`:/`pwd`:ro -w /`pwd` golangci/golangci-lint:v1.39.0-alpine golangci-lint run --deadline=5m -v

build:
	go build --ldflags "-s -w -linkmode external -extldflags -static -X main.version=$(CI_BRANCH)" --tags netcgo -o ./bin/server ./cmd/server/
	cp -r ./auxiliary ./bin/

build_mac:
	go build -o ./bin/main ./cmd/server/
	go build -o ./bin/tradehistory ./tools/history/
	cp -r ./auxiliary ./bin/

build_docker:
	docker build --tag=AnotherDevvv/pdax-monitor:latest --file=docker/Dockerfile .

up:
	docker-compose -f docker/docker-compose.yml up -d --build

down:
	docker-compose -f docker/docker-compose.yml down
