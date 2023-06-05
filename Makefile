.PYONY: build-linux-amd64
build-linux-amd64: 
	GOOS=linux GOARCH=amd64 go build -o logbook

.PYONY: build-win-amd64
build-win-amd64:
	GOOS=windows GOARCH=amd64 go build -o logbook.exe
