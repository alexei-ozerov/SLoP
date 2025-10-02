package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/jeffry-luqman/zlog"
)

type ParserContext struct {
	Filter string
	Level  string
	Pretty bool
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
	var firstLineRegexp = regexp.MustCompile(`^(?P<time>\d{4}-\d{1,2}-\d{1,2}T\d{1,2}:\d{1,2}:\d{1,2}[,.]\d{3}(?:Z|[+-]\d{2}:\d{2})?)\s+(?P<level>[A-Z]+)\s+(?P<pid>\d+)\s+---\s+\[(?P<thread>.*?)\]\s+(?P<class>.*?)\s*:\s*(?P<message>.*)`)
	var continuedRegexp = regexp.MustCompile(`^\s+at\s+.*|^\s*Caused by:.*`)

	match := firstLineRegexp.FindStringSubmatch(line)

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
			Message:     strings.TrimSpace(match[6]),
			Raw:         line,
			ParseStatus: 1, // First line deserialized
		}

		return &springLogLine, nil
	}

	if logObjectBuffer.ParseStatus > 0 && (logObjectBuffer.Level == "ERROR" || logObjectBuffer.Level == "WARN" || continuedRegexp.MatchString(line)) {
		if continuedRegexp.MatchString(line) || strings.HasPrefix(line, "\t") || (len(line) > 0 && line[0] == ' ') {
			logObjectBuffer.Raw += "\n" + line
			logObjectBuffer.Message += "\n" + line
			logObjectBuffer.ParseStatus = 2 // Log appended
			return logObjectBuffer, nil
		}
	}

	return nil, nil
}

func readStdin(ctx ParserContext) error {
	var logObjectBuffer SpringLogStruct
	var previousLogObject *SpringLogStruct
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}

		bufferBeforeProcessing := logObjectBuffer

		lineObj, err := processLogLine(line, &logObjectBuffer)
		if err != nil {
			return err
		}

		if logObjectBuffer.ParseStatus == 3 {
			var previousTimestamp string
			if previousLogObject != nil {
				previousTimestamp = previousLogObject.Time
			}
			if err := printResults(ctx, &bufferBeforeProcessing, previousTimestamp); err != nil {
				return err
			}
			prev := bufferBeforeProcessing
			previousLogObject = &prev
		}

		if lineObj != nil {
			logObjectBuffer = *lineObj
		}
	}

	if logObjectBuffer.ParseStatus != 0 {
		var previousTimestamp string
		if previousLogObject != nil {
			previousTimestamp = previousLogObject.Time
		}
		if err := printResults(ctx, &logObjectBuffer, previousTimestamp); err != nil {
			return err
		}
	}

	return scanner.Err()
}

func parseTime(timestamp string) (time.Time, error) {
	normalizedTimestamp := strings.Replace(timestamp, ",", ".", 1)

	return time.Parse(time.RFC3339Nano, normalizedTimestamp)
}

func checkFilter(ctx ParserContext, logObject *SpringLogStruct) bool {
	if strings.Contains(logObject.Raw, ctx.Filter) {
		return true
	}

	return false
}

func checkLevel(ctx ParserContext, logObject *SpringLogStruct) bool {
	if logObject.Level == ctx.Level {
		return true
	}

	return false
}

func printResults(ctx ParserContext, logObject *SpringLogStruct, previousTimestamp string) error {
	filterPassed := true

	// If level filter exists
	if ctx.Level != "" {
		// Only print object if it matches filter
		filterPassed = checkLevel(ctx, logObject)

		if filterPassed == false {
			return nil
		}

		if ctx.Filter != "" {
			// Only print object if it matches filter
			filterPassed = checkFilter(ctx, logObject)
		}
	}

	if ctx.Filter != "" && ctx.Level == "" {
		// Only print object if it matches filter
		filterPassed = checkFilter(ctx, logObject)
	}

	if filterPassed {
		if err := prettyPrintJson(logObject, &ctx, previousTimestamp); err != nil {
			return err
		}
	}

	return nil
}

func prettyPrintJson(lineObj *SpringLogStruct, ctx *ParserContext, previousTimestamp string) error {
	logObject, err := json.Marshal(lineObj)
	if err != nil {
		return err
	}

	var indentedJson bytes.Buffer
	indentErr := json.Indent(&indentedJson, logObject, "", "\t")
	if indentErr != nil {
		return indentErr
	}

	if ctx.Pretty {
		switch lineObj.Level {
		case "ERROR":
			color.Red("LEVEL:   " + lineObj.Level)
		case "WARN":
			color.Yellow("LEVEL:   " + lineObj.Level)
		case "INFO":
			color.Green("LEVEL:   " + lineObj.Level)
		default: 
			fmt.Println("LEVEL:   " + lineObj.Level)
		}

		var diff time.Duration
		var diffString string
		if previousTimestamp != "" {
			p, pErr := parseTime(previousTimestamp)
			t, tErr := parseTime(lineObj.Time)

			if pErr == nil && tErr == nil {
				diff = t.Sub(p)
				minutes := int(diff.Minutes())
				seconds := int(diff.Seconds()) % 60
				diffString = fmt.Sprintf("%d minutes and %d seconds", minutes, seconds)
			} else {
				if pErr != nil {
					slog.Error("Error parsing previous timestamp", "timestamp", previousTimestamp, "error", pErr)
				}
				if tErr != nil {
					slog.Error("Error parsing current timestamp", "timestamp", lineObj.Time, "error", tErr)
				}
				diffString = "0 minutes and 0 seconds"
			}
		}

		fmt.Println("TIME:    " + lineObj.Time)
		fmt.Println("DIFF:    " + diffString)
		fmt.Println("THREAD:  " + lineObj.Thread)
		fmt.Println("CLASS:   " + lineObj.Class)
		fmt.Println("MESSAGE: " + lineObj.Message)
		fmt.Println("")
	} else {
		fmt.Println(indentedJson.String())
	}

	return nil
}

func setupAppDir() error {
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(homeDirectory, ".slop")
	createErr := os.MkdirAll(configPath, os.ModePerm)
	if createErr != nil {
		return createErr
	}

	return nil
}

func main() {
	// Setup logger
	logger := zlog.New()
	slog.SetDefault(logger)

	// Setup Application Directory
	setupErr := setupAppDir()
	if setupErr != nil {
		slog.Error("Error encountered setting up application directory.", "Error", setupErr)
	}

	// Parse cli opts & construct config struct
	levelFilter := flag.String("level", "", "Log level you want to filter for")
	contentFilter := flag.String("grep", "", "Search term you want to filter for")
	prettyPrint := flag.Bool("pretty", false, "Turn on pretty printing (invalid JSON output, but pretty :3)")
	flag.Parse()

	// TODO: Add configuration values from local config here
	options := ParserContext{
		Filter: *contentFilter,
		Level:  *levelFilter,
		Pretty: *prettyPrint,
	}

	// Parse Stdin
	err := readStdin(options)
	if err != nil {
		slog.Error("Encountered error: ", "ErrorMessage", err)
		os.Exit(1)
	}
}
