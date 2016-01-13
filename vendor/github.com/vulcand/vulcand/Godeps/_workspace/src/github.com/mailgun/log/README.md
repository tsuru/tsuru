log
===

Go logging library used at Mailgun.

Usage
-----

The mailgun/log package supports chains of loggers where the same message can go to multiple logging channels simultaneously, for example, the standard output and syslog.

Currently, the following loggers are supported: console (stdout), syslog and updlog. The latter requires having udplog server (https://github.com/mochi/udplog) running locally. Custom loggers can implement the package's `Logger` interface and be intergated into the logger chain.

Before using the package it should be initialized at the start of a program. It can be done in two ways.

**Initialize with loggers**

```go
import "github.com/mailgun/log"

func main() {
  // create a console logger
  console, _ := log.NewLogger(log.Config{"console", "info"})

  // create a syslogger
  syslog, _ := log.NewLogger(log.Config{"syslog", "error"})

  // init the logging package
  log.Init(console, syslog) // or any other logger implementing `log.Logger` can be provided
}
```

**Initialize with a config**

Mailgun's cfg package (https://github.com/mailgun/cfg) simplifies this method.

Define a logging configuration in a YAML config file.

```yaml
logging:
  - name:     console
    severity: error
  - name:     syslog
    severity: info
```

Logging config can be built into your program's config struct:

```go
import "github.com/mailgun/log"

type Config struct {
  // some program-specific configuration

  // logging configuration
  Logging []log.Config
}
```

After config parsing, initialize the logging library:

```go
import (
  "github.com/mailgun/cfg"
  "github.com/mailgun/log"
)

func main() {
  conf := Config{}

  // parse config with logging configuration
  cfg.LoadConfig("path/to/config.yaml", &conf)

  // init the logging package
  log.InitWithConfig(conf.Logging...)
}
```
