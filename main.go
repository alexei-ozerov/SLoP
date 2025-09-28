package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/jeffry-luqman/zlog"
)

type ParserContext struct {
	Filter string
	Level  string
}

type SpringLogStruct struct {
	Time        string
	Level       string
	Pid         string
	Thread      string
	Class       string
	Message     string
	Raw         string
	ParseStatus int
}

func processLogLine(line string, logObjectBuffer *SpringLogStruct) (logLine *SpringLogStruct, err error) {
	var firstLineRegexp = regexp.MustCompile(`^(?P<time>\d{4}-\d{1,2}-\d{1,2}T\d{1,2}:\d{1,2}:\d{1,2}.\d{3}Z)\s+(?P<level>[^\s]+)\s+(?P<pid>\d+).*?\[\s+(?P<thread>.*)\]\s+(?P<class>.*)\s+:\s+(?P<message>.*)`)
	var continuedRegexp = regexp.MustCompile(`^\s+at\s+.*`)

	match := firstLineRegexp.FindStringSubmatch(line)

	// We've found a match for the expected first line of a SpringBoot log
	if len(match) > 0 {
		if logObjectBuffer.ParseStatus != 0 {
			logObjectBuffer.ParseStatus = 3 // Previous line completed
		}

		springLogLine := SpringLogStruct{
			Time:        strings.Join(strings.Fields(match[1]), " "),
			Level:       strings.Join(strings.Fields(match[2]), " "),
			Pid:         strings.Join(strings.Fields(match[3]), " "),
			Thread:      strings.Join(strings.Fields(match[4]), " "),
			Class:       strings.Join(strings.Fields(match[5]), " "),
			Message:     strings.Join(strings.Fields(match[6]), " "),
			Raw:         match[0],
			ParseStatus: 1, // First line deserialized
		}

		return &springLogLine, nil
	} else {
		match := continuedRegexp.MatchString(line)
		if match {
			springLogLine := *logObjectBuffer
			springLogLine.ParseStatus = 2 // Log appended
			springLogLine.Raw += line

			return &springLogLine, nil
		}

		return nil, nil
	}
}

func readStdin(ctx ParserContext) error {
	var logObjectBuffer SpringLogStruct
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			break
		}

		// Get serialized log data
		lineObj, err := processLogLine(line, &logObjectBuffer)
		if err != nil {
			return err
		}

		// If log buffer status is 3, this means the object is complete and ready to print
		if logObjectBuffer.ParseStatus == 3 {
			if err := printResults(ctx, &logObjectBuffer); err != nil {
				return err
			}

			// Reset the buffer
			logObjectBuffer = SpringLogStruct{}
		}

		if lineObj != nil {
			logObjectBuffer = *lineObj
		}
	}

	if logObjectBuffer.ParseStatus != 0 {
		logObjectBuffer.ParseStatus = 3 // Mark as completed for printing
		if err := printResults(ctx, &logObjectBuffer); err != nil {
			return err
		}
	}

	return scanner.Err()
}

func printResults(ctx ParserContext, logObject *SpringLogStruct) error {
	// If level filter exists
	if ctx.Level != "" {
		// Only print object if it matches filter
		if logObject.Level == ctx.Level {
			if err := prettyPrintJson(logObject); err != nil {
				return err
			}
		}
	} else {
		// If no level filter set, print object
		if err := prettyPrintJson(logObject); err != nil {
			return err
		}
	}

	return nil
}

func prettyPrintJson(lineObj *SpringLogStruct) error {
	logObject, err := json.Marshal(lineObj)
	if err != nil {
		return err
	}

	var prettyJSON bytes.Buffer
	indentErr := json.Indent(&prettyJSON, logObject, "", "\t")
	if indentErr != nil {
		return indentErr
	}
	slog.Info("Line deserialized: ", "JsonObject", prettyJSON.String())

	return nil
}

func main() {
	// Setup logger
	logger := zlog.New()
	slog.SetDefault(logger)

	// Parse Cli Opts
	levelFilter := flag.String("level", "", "Log level you want to filter for")
	flag.Parse()

	options := ParserContext{
		Filter: "",
		Level:  *levelFilter,
	}

	// Parse Stdin
	err := readStdin(options)
	if err != nil {
		slog.Error("Encountered error: ", "ErrorMessage", err)
		os.Exit(1)
	}
}
