package worker

type ILogger interface {
	LogTrace(format string, v ...interface{})
	LogDebug(format string, v ...interface{})
	LogInfo(format string, v ...interface{})
	LogWarn(format string, v ...interface{})
	LogError(format string, v ...interface{})
	LogStack(format string, v ...interface{})
	LogFatal(format string, v ...interface{})
}
