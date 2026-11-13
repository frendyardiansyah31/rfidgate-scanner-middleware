package logger

type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)
}

type Field struct {
	Key   string
	Value any
}

func F(key string, value any) Field {
	return Field{Key: key, Value: value}
}
