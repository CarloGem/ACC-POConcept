package logger

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	log "github.com/sirupsen/logrus"
)

func Init() {
	// Set log format
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
		FullTimestamp:   true,
		ForceColors:     true,
	})

	// Set log output
	log.SetOutput(os.Stdout)

	// Set log level
	log.SetLevel(log.InfoLevel)
}

func GetLogger() *log.Logger {
	return log.StandardLogger()
}

// Info logs an informational message with green color formatting
func Info(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...) // Use fmt.Sprintf to format the message
	color.Green("[INFO] %s", message)
	log.Info(message)
}

// Warn logs a warning message with yellow color formatting
func Warn(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...) // Use fmt.Sprintf to format the message
	color.Yellow("[WARN] %s", message)
	log.Warn(message)
}

// Error logs an error message with red color formatting
func Error(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...) // Use fmt.Sprintf to format the message
	color.Red("[ERROR] %s", message)
	log.Error(message)
}

// Debug logs a debug message with cyan color formatting
func Debug(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...) // Use fmt.Sprintf to format the message
	color.Cyan("[DEBUG] %s", message)
	log.Debug(message)
}
