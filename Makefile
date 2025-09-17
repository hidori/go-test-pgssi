.PHONY: clean
clean:
	rm -rf tmp

.PHONY: install
install:
	mise install

.PHONY: build
build:
	mkdir -p tmp
	go build -o tmp/go-test-pgssi

.PHONY: run
run:
	mkdir -p tmp
	go run main.go 2>&1 | tee tmp/run.log

.PHONY: compose/up
compose/up: compose/down
	docker-compose up -d

.PHONY: compose/down
compose/down:
	docker-compose down
