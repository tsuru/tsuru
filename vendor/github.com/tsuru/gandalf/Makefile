BUILD_DIR = build
GANDALF_WEBSERVER_BIN = $(BUILD_DIR)/gandalf-webserver
GANDALF_WEBSERVER_SRC = webserver/main.go
GANDALF_SSH_BIN = $(BUILD_DIR)/gandalf-ssh
GANDALF_SSH_SRC = bin/gandalf.go

test:
	go clean $(GO_EXTRAFLAGS) ./...
	go test $(GO_EXTRAFLAGS) ./...

doc:
	@cd docs && make html

binaries: gandalf-webserver gandalf-ssh

gandalf-webserver: $(GANDALF_WEBSERVER_BIN)

$(GANDALF_WEBSERVER_BIN):
	go build -o $(GANDALF_WEBSERVER_BIN) $(GANDALF_WEBSERVER_SRC)

run-gandalf-webserver: $(GANDALF_WEBSERVER_BIN)
	$(GANDALF_WEBSERVER_BIN) $(GANDALF_WEBSERVER_OPTIONS)

gandalf-ssh: $(GANDALF_SSH_BIN)

$(GANDALF_SSH_BIN):
	go build -o $(GANDALF_SSH_BIN) $(GANDALF_SSH_SRC)

run-gandalf-ssh: $(GANDALF_SSH_BIN)
	$(GANDALF_SSH_BIN) $(GANDALF_SSH_OPTIONS)
