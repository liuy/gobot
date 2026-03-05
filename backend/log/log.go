package log

import (
	"fmt"
	stdlog "log"
	"os"
)

var (
	debug = os.Getenv("GOBOT_DEBUG") == "1"
	color = os.Getenv("GOBOT_NO_COLOR") != "1"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

func Info(format string, args ...any) {
	print("INFO", colorGreen, format, args...)
}

func Debug(format string, args ...any) {
	if debug {
		print("DEBUG", colorCyan, format, args...)
	}
}

func Error(format string, args ...any) {
	print("ERROR", colorRed, format, args...)
}

func Warn(format string, args ...any) {
	print("WARN", colorYellow, format, args...)
}

func Fatal(format string, args ...any) {
	print("FATAL", colorRed, format, args...)
	os.Exit(1)
}

func print(level, colorCode, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if color {
		stdlog.Printf("%s[%s]%s %s", colorCode, level, colorReset, msg)
	} else {
		stdlog.Printf("[%s] %s", level, msg)
	}
}
