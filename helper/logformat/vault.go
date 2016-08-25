package logformat

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/jefferai/logxi/v1"
)

const (
	styledefault = iota
	stylejson
)

// NewVaultLogger creates a new logger with the specified level and a Vault
// formatter
func NewVaultLogger(level int) log.Logger {
	logger := log.New("vault")
	return setLevelFormatter(logger, level, createVaultFormatter())
}

// NewVaultLoggerWithWriter creates a new logger with the specified level and
// writer and a Vault formatter
func NewVaultLoggerWithWriter(w io.Writer, level int) log.Logger {
	logger := log.NewLogger(w, "vault")
	return setLevelFormatter(logger, level, createVaultFormatter())
}

// Sets the level and formatter on the log, which must be a DefaultLogger
func setLevelFormatter(logger log.Logger, level int, formatter log.Formatter) log.Logger {
	logger.(*log.DefaultLogger).SetLevel(level)
	logger.(*log.DefaultLogger).SetFormatter(formatter)
	return logger
}

// DeriveModuleLogger derives  a logger from the input logger and the given
// module string. A derived logger shares the underlying writer, level, and
// style, but the module set on the formatter is different. If the string is
// empty, it derives a new logger with no module set; if the string begins with
// "/" it sets the following name as the module name itself (rather than
// deriving one with no module and then deriving a new one).
func DeriveModuleLogger(logger log.Logger, module string) log.Logger {
	defLogger := logger.(*log.DefaultLogger)
	formatter := defLogger.Formatter().(*vaultFormatter)
	newFormatter := &vaultFormatter{
		Mutex: formatter.Mutex,
		style: formatter.style,
	}
	switch {
	case module == "":
		// Don't set a module, clear it instead
	case strings.HasPrefix(module, "/"):
		newFormatter.module = module[1:]
	case formatter.module == "":
		newFormatter.module = module
	default:
		newFormatter.module = fmt.Sprintf("%s/%s", formatter.module, module)
	}

	newLogger := log.NewLogger(defLogger.Writer(), "vault")
	return setLevelFormatter(newLogger, defLogger.Level(), newFormatter)
}

// Creates a formatter, checking env vars for the style
func createVaultFormatter() log.Formatter {
	ret := &vaultFormatter{
		Mutex: &sync.Mutex{},
	}
	switch os.Getenv("LOGXI_FORMAT") {
	case "vault_json", "vault-json", "vaultjson":
		ret.style = stylejson
	default:
		ret.style = styledefault
	}
	return ret
}

// Thread safe formatter
type vaultFormatter struct {
	*sync.Mutex
	style  int
	module string
}

func (v *vaultFormatter) Format(writer io.Writer, level int, msg string, args []interface{}) {
	v.Lock()
	defer v.Unlock()
	switch v.style {
	case stylejson:
		v.formatJSON(writer, level, msg, args)
	default:
		v.formatDefault(writer, level, msg, args)
	}
}

func (v *vaultFormatter) formatDefault(writer io.Writer, level int, msg string, args []interface{}) {
	// Write a trailing newline
	defer writer.Write([]byte("\n"))

	writer.Write([]byte(time.Now().Local().Format("2006/01/02 15:04:05.000000")))

	switch level {
	case log.LevelCritical:
		writer.Write([]byte(" [CRT] "))
	case log.LevelError:
		writer.Write([]byte(" [ERR] "))
	case log.LevelWarn:
		writer.Write([]byte(" [WRN] "))
	case log.LevelInfo:
		writer.Write([]byte(" [INF] "))
	case log.LevelDebug:
		writer.Write([]byte(" [DBG] "))
	case log.LevelTrace:
		writer.Write([]byte(" [TRC] "))
	default:
		writer.Write([]byte(" [ALL] "))
	}

	if v.module != "" {
		writer.Write([]byte(fmt.Sprintf("(%s) ", v.module)))
	}

	writer.Write([]byte(msg))

	if args != nil && len(args) > 0 {
		if len(args)%2 != 0 {
			args = append(args, "[unknown!]")
		}

		writer.Write([]byte(":"))

		for i := 0; i < len(args); i = i + 2 {
			var quote string
			switch args[i+1].(type) {
			case string:
				if strings.ContainsRune(args[i+1].(string), ' ') {
					quote = `"`
				}
			}
			writer.Write([]byte(fmt.Sprintf(" %s=%s%v%s", args[i], quote, args[i+1], quote)))
		}
	}
}

func (v *vaultFormatter) formatJSON(writer io.Writer, level int, msg string, args []interface{}) {
	vals := map[string]interface{}{
		"@message":   msg,
		"@timestamp": time.Now().Format("2006-01-02T15:04:05.000000Z07:00"),
	}

	var levelStr string
	switch level {
	case log.LevelCritical:
		levelStr = "critical"
	case log.LevelError:
		levelStr = "error"
	case log.LevelWarn:
		levelStr = "warn"
	case log.LevelInfo:
		levelStr = "info"
	case log.LevelDebug:
		levelStr = "debug"
	case log.LevelTrace:
		levelStr = "trace"
	default:
		levelStr = "all"
	}

	vals["@level"] = levelStr

	if v.module != "" {
		vals["@module"] = v.module
	}

	if args != nil && len(args) > 0 {

		if len(args)%2 != 0 {
			args = append(args, "[unknown!]")
		}

		for i := 0; i < len(args); i = i + 2 {
			if _, ok := args[i].(string); !ok {
				// As this is the logging function not much we can do here
				// without injecting into logs...
				continue
			}
			vals[args[i].(string)] = args[i+1]
		}
	}

	enc := json.NewEncoder(writer)
	enc.Encode(vals)
}