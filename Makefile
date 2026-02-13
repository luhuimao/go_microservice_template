run:
	go run cmd/api/main.go

build:
	go build -o app cmd/api/main.go

docker:
	docker-compose up --build
