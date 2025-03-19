package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logger zerolog.Logger

// Info writes record into os.stdout with log level INFO
func Info(v ...interface{}) {
	if len(v) == 1 {
		logger.Info().Interface("message", v[0]).Send()
	} else {
		logger.Info().Msgf("%s", v...)
	}
}

// Info writes record into os.stdout with log level INFO
func Infof(format string, v ...interface{}) {
	logger.Info().Msgf(format, v...)
}

// Debug writes record into os.stdout with log level DEBUG
func Debug(v ...interface{}) {
	logger.Debug().Msgf("%s", v...)
}

// Debugf writes record into os.stdout with log level DEBUG
func Debugf(format string, v ...interface{}) {
	logger.Debug().Msgf(format, v...)
}

// Error writes record into os.stdout with log level ERROR
func Error(v ...interface{}) {
	logger.Error().Msgf("%s", v...)
}

// Fatal writes record into os.stdout with log level ERROR and exits
func Fatal(v ...interface{}) {
	logger.Fatal().Msgf("%s", v...)
	os.Exit(1)
}

// Fatal writes record into os.stdout with log level ERROR
func Fatalf(format string, v ...interface{}) {
	logger.Fatal().Msgf(format, v...)
	os.Exit(1)
}

// Error writes record into os.stdout with log level ERROR
func Errorf(format string, v ...interface{}) {
	logger.Error().Msgf(format, v...)
}

// Warn writes record into os.stdout with log level WARN
func Warn(v ...interface{}) {
	logger.Warn().Msgf("%s", v...)
}

// Warn writes record into os.stdout with log level WARN
func Warnf(format string, v ...interface{}) {
	logger.Warn().Msgf(format, v...)
}

func LogResponse(response *http.Response) {
	respDump, err := httputil.DumpResponse(response, true)
	if err != nil {
		Fatal(err)
	}

	fmt.Println(string(respDump))
}

func LogRequest(req *http.Request) {
	requestDump, err := httputil.DumpRequest(req, true)
	if err != nil {
		Fatal(err)
	}

	fmt.Println(string(requestDump))
}

// CreateFile creates a new file or overwrites an existing one with the specified filename, path, extension,
func FileLogger(content any, fileName, fileExtension string) error {
	// get config folder
	filePath := viper.GetString("CONFIG_FOLDER")
	if filePath == "" {
		return fmt.Errorf("config folder is not set")
	}
	// Construct the full file path
	contentBytes, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("failed to marshal content: %s", err)
	}

	fullPath := filepath.Join(filePath, fileName+fileExtension)

	// Create or truncate the file
	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create or open file: %s", err)
	}
	defer file.Close()

	// Write data to the file
	_, err = file.Write(contentBytes)
	if err != nil {
		return fmt.Errorf("failed to write data to file: %s", err)
	}

	return nil
}

func StatsLogger(ctx context.Context, statsFunc func() (int64, int64, int64)) {
	startTime := time.Now()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				Info("Monitoring stopped")
				return
			case <-ticker.C:
				syncedRecords, runningThreads, recordsToSync := statsFunc()
				memStats := new(runtime.MemStats)
				runtime.ReadMemStats(memStats)
				speed := float64(syncedRecords) / time.Since(startTime).Seconds()
				timeElapsed := time.Since(startTime).Seconds()
				remainingRecords := recordsToSync - syncedRecords
				estimatedSeconds := "Not Determined"
				if speed > 0 && remainingRecords >= 0 {
					estimatedSeconds = fmt.Sprintf("%.2f s", float64(remainingRecords)/speed)
				}
				stats := map[string]interface{}{
					"Running Threads":          runningThreads,
					"Synced Records":           syncedRecords,
					"Memory":                   fmt.Sprintf("%d mb", memStats.HeapInuse/(1024*1024)),
					"Speed":                    fmt.Sprintf("%.2f rps", speed),
					"Seconds Elapsed":          fmt.Sprintf("%.2f", timeElapsed),
					"Estimated Remaining Time": estimatedSeconds,
				}
				if err := FileLogger(stats, "stats", ".json"); err != nil {
					Fatalf("failed to write stats in file: %s", err)
				}
			}
		}
	}()
}

func Init() {
	// Configure lumberjack for log rotation
	currentTimestamp := time.Now().UTC()
	timestamp := fmt.Sprintf("%d-%02d-%02d_%02d-%02d-%02d", currentTimestamp.Year(), currentTimestamp.Month(), currentTimestamp.Day(), currentTimestamp.Hour(), currentTimestamp.Minute(), currentTimestamp.Second())
	rotatingFile := &lumberjack.Logger{
		Filename:   fmt.Sprintf("%s/logs/sync_%s/olake.log", viper.GetString("CONFIG_FOLDER"), timestamp), // Log file path
		MaxSize:    100,                                                                                   // Max size in MB before log rotation
		MaxBackups: 5,                                                                                     // Max number of old log files to retain
		MaxAge:     30,                                                                                    // Max age in days to retain old log files
		Compress:   true,                                                                                  // Compress old log files
	}
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().UTC()
	}
	var currentLevel string
	// LogColors defines ANSI color codes for log levels
	var logColors = map[string]string{
		"debug": "\033[36m", // Cyan
		"info":  "\033[32m", // Green
		"warn":  "\033[33m", // Yellow
		"error": "\033[31m", // Red
		"fatal": "\033[31m", // Red
	}
	// Create console writer
	console := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02 15:04:05",
		FormatLevel: func(i interface{}) string {
			level := i.(string)
			currentLevel = level
			color := logColors[level]
			return fmt.Sprintf("%s%s\033[0m", color, strings.ToUpper(level))
		},
		FormatMessage: func(i interface{}) string {
			msg := ""
			switch v := i.(type) {
			case string:
				msg = v
			default:
				jsonMsg, err := json.Marshal(v)
				if err != nil {
					Errorf("failed to marshal log message: %s", err)
					return err.Error()
				}
				return string(jsonMsg)
			}
			// Get the current log level from the context
			if currentLevel == zerolog.ErrorLevel.String() || currentLevel == zerolog.FatalLevel.String() {
				msg = fmt.Sprintf("\033[31m%s\033[0m", msg) // Make entire message red for error level
			}
			return msg
		},
		FormatTimestamp: func(i interface{}) string {
			return fmt.Sprintf("\033[90m%s\033[0m", i)
		},
	}
	// Create a multiwriter to log both console and file
	multiwriter := zerolog.MultiLevelWriter(console, rotatingFile)

	logger = zerolog.New(multiwriter).With().Timestamp().Logger()
}
