build:
	rm -f main && go build cmd/main.go

docker:
	docker build -t jarmex/open-match-director .

push:
	docker push jarmex/open-match-director
