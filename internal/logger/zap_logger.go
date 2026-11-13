package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type ZapLogger struct {
	log *zap.Logger
}

// func New(level string) (*ZapLogger, error) {
// 	zapLevel := zapcore.InfoLevel

// 	if err := zapLevel.Set(level); err != nil {
// 		return nil, err
// 	}

// 	cfg := zap.Config{
// 		Level:       zap.NewAtomicLevelAt(zapLevel),
// 		Development: false,
// 		Encoding:    "json",
// 		EncoderConfig: zapcore.EncoderConfig{
// 			TimeKey:      "ts",
// 			LevelKey:     "level",
// 			MessageKey:   "msg",
// 			EncodeTime:   zapcore.ISO8601TimeEncoder,
// 			EncodeLevel:  zapcore.LowercaseLevelEncoder,
// 			CallerKey:    "caller",
// 			EncodeCaller: zapcore.ShortCallerEncoder,
// 		},
// 		OutputPaths:      []string{"stdout"},
// 		ErrorOutputPaths: []string{"stderr"},
// 	}

// 	log, err := cfg.Build(zap.AddCaller())
// 	if err != nil {
// 		return nil, err
// 	}

//		return &ZapLogger{log: log}, nil
//	}
// func New(level string) (*ZapLogger, error) {
// 	_ = os.MkdirAll("logs", 0755)

// 	atomicLevel := zap.NewAtomicLevel()
// 	if err := atomicLevel.UnmarshalText([]byte(level)); err != nil {
// 		return nil, err
// 	}

// 	encoderCfg := zapcore.EncoderConfig{
// 		TimeKey:      "ts",
// 		LevelKey:     "level",
// 		MessageKey:   "msg",
// 		CallerKey:    "caller",
// 		EncodeTime:   zapcore.ISO8601TimeEncoder,
// 		EncodeLevel:  zapcore.LowercaseLevelEncoder,
// 		EncodeCaller: zapcore.ShortCallerEncoder,
// 	}

// 	fileWriter := zapcore.AddSync(&lumberjack.Logger{
// 		Filename:   "logs/rfid-middleware.log",
// 		MaxSize:    50,
// 		MaxBackups: 10,
// 		MaxAge:     14,
// 		Compress:   true,
// 	})

// 	consoleWriter := zapcore.AddSync(os.Stdout)

// 	core := zapcore.NewTee(
// 		zapcore.NewCore(
// 			zapcore.NewJSONEncoder(encoderCfg),
// 			fileWriter,
// 			atomicLevel,
// 		),
// 		zapcore.NewCore(
// 			zapcore.NewConsoleEncoder(encoderCfg),
// 			consoleWriter,
// 			atomicLevel,
// 		),
// 	)

// 	log := zap.New(core, zap.AddCaller())

// 	return &ZapLogger{log: log}, nil
// }

// func (z *ZapLogger) Debug(msg string, fields ...Field) {
// 	z.log.Debug(msg, toZapFields(fields)...)
// }

// func (z *ZapLogger) Info(msg string, fields ...Field) {
// 	z.log.Info(msg, toZapFields(fields)...)
// }

// func (z *ZapLogger) Warn(msg string, fields ...Field) {
// 	z.log.Warn(msg, toZapFields(fields)...)
// }

// func (z *ZapLogger) Error(msg string, fields ...Field) {
// 	z.log.Error(msg, toZapFields(fields)...)
// }

// func (z *ZapLogger) Fatal(msg string, fields ...Field) {
// 	z.log.Fatal(msg, toZapFields(fields)...)
// 	// os.Exit(1)
// }

// func toZapFields(fields []Field) []zap.Field {
// 	zf := make([]zap.Field, 0, len(fields))
// 	for _, f := range fields {
// 		zf = append(zf, zap.Any(f.Key, f.Value))
// 	}
// 	return zf
// }

// New builds the app logger. baseDir anchors the "logs" folder to the
// executable's directory (same treatment as SQLite path in config.Load) so a
// Windows service installed via kardianos/service — whose CWD is
// %SystemRoot%\System32, not the exe's folder — still writes next to the
// binary instead of into System32\logs. Pass "" to use the process's CWD
// (e.g. `go run`/interactive use).
func New(level string, baseDir string) (*ZapLogger, error) {
	logDir := "logs"
	if baseDir != "" {
		logDir = filepath.Join(baseDir, "logs")
	}

	// 1. Pastikan folder logs ada
	_ = os.MkdirAll(logDir, 0755)

	// 2. Parse Level Log (Debug, Info, Error, dll)
	atomicLevel := zap.NewAtomicLevel()
	if err := atomicLevel.UnmarshalText([]byte(level)); err != nil {
		return nil, err
	}

	// 3. Konfigurasi Encoder (Format Tampilan)

	// A. Config untuk File (JSON, Standar, Lowercase)
	fileEncoderCfg := zap.NewProductionEncoderConfig()
	fileEncoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	// B. Config untuk Console (Text, Berwarna)
	// FIX: Langsung pakai return valuenya, tidak perlu .EncoderConfig lagi
	consoleEncoderCfg := zap.NewDevelopmentEncoderConfig()
	consoleEncoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	consoleEncoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder // 🔥 Warna di terminal

	// 4. Konfigurasi Log Rotation (Lumberjack)
	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   filepath.Join(logDir, "rfid-middleware.log"),
		MaxSize:    50, // Megabytes
		MaxBackups: 10,
		MaxAge:     14,   // Days
		Compress:   true, // .gz
	})

	consoleWriter := zapcore.AddSync(os.Stdout)

	// 5. Multi-Output Core (Tee)
	core := zapcore.NewTee(
		// Output ke File: Format JSON
		zapcore.NewCore(
			zapcore.NewJSONEncoder(fileEncoderCfg),
			fileWriter,
			atomicLevel,
		),
		// Output ke Console: Format Text Berwarna
		zapcore.NewCore(
			zapcore.NewConsoleEncoder(consoleEncoderCfg),
			consoleWriter,
			atomicLevel,
		),
	)

	// 6. Build Logger
	// AddCallerSkip(1) agar log menunjuk ke pemanggil (main.go), bukan wrapper ini
	log := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))

	return &ZapLogger{log: log}, nil
}

// --- Wrapper Methods ---

func (z *ZapLogger) Debug(msg string, fields ...Field) {
	z.log.Debug(msg, toZapFields(fields)...)
}

func (z *ZapLogger) Info(msg string, fields ...Field) {
	z.log.Info(msg, toZapFields(fields)...)
}

func (z *ZapLogger) Warn(msg string, fields ...Field) {
	z.log.Warn(msg, toZapFields(fields)...)
}

func (z *ZapLogger) Error(msg string, fields ...Field) {
	z.log.Error(msg, toZapFields(fields)...)
}

func (z *ZapLogger) Fatal(msg string, fields ...Field) {
	z.log.Fatal(msg, toZapFields(fields)...)
}

func toZapFields(fields []Field) []zap.Field {
	zf := make([]zap.Field, 0, len(fields))
	for _, f := range fields {
		zf = append(zf, zap.Any(f.Key, f.Value))
	}
	return zf
}
