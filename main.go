package main

import (
	"github.com/wdxxs2z/skywalking-osb/broker"
	"code.cloudfoundry.org/lager"
	"flag"
	"strings"
	"log"
	"os"
)

var (
	configpath 	string
	port           	string

	logLevels = map[string]lager.LogLevel{
		"DEBUG": lager.DEBUG,
		"INFO":  lager.INFO,
		"ERROR": lager.ERROR,
		"FATAL": lager.FATAL,
	}
)

func init() {
	flag.StringVar(&configpath, "config", "", "The helmi config path")
	flag.StringVar(&port, "port", "8080", "Listen port")
}

func buildLogger(logLevel string) lager.Logger {
	laggerLogLevel, ok := logLevels[strings.ToUpper(logLevel)]
	if !ok {
		log.Fatal("Invalid log level: ", logLevel)
	}

	logger := lager.NewLogger("skywalking-service-broker")
	logger.RegisterSink(lager.NewWriterSink(os.Stdout, laggerLogLevel))

	return logger
}

func main() {
	flag.Parse()
	config, err := LoadConfig(configpath)

	if err != nil {
		log.Fatalf("Error loading config file: %s", err)
	}

	logger := buildLogger(config.LogLevel)

	os.Setenv("USERNAME", config.Username)
	os.Setenv("PASSWORD", config.Password)

	broker := broker.New(config.ServiceConfig, logger)
	broker.Run(":" + port)
}
