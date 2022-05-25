package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/gogo/protobuf/proto"
	"github.com/segmentio/encoding/json"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type defaultLogger struct {
	// used by options
	writers     []io.Writer // sys and tdr mus have different channel of writer
	maskEnabled bool
	noopLogger  bool
	closer      []io.Closer

	// initiated by this application newLogger
	zapLogger *zap.Logger
	level     Level
}

var _ Logger = (*defaultLogger)(nil)

func newLogger(opts ...Option) (Logger, error) {
	defaultLogger := &defaultLogger{
		writers:     make([]io.Writer, 0),
		maskEnabled: false,
	}

	for _, o := range opts {
		if err := o(defaultLogger); err != nil {
			return nil, err
		}
	}

	// set logger here instead in options to make easy and consistent initiation
	// set multiple writer as already set in options
	defaultLogger.zapLogger = NewZapLogger(defaultLogger.level, defaultLogger.writers...)

	// use stdout only when writer is not specified
	if len(defaultLogger.writers) <= 0 {
		defaultLogger.zapLogger = NewZapLogger(defaultLogger.level, zapcore.AddSync(os.Stdout))
	}

	// if noop logger enabled, then use discard all print
	if defaultLogger.noopLogger {
		defaultLogger.zapLogger = zap.NewNop()
	}

	return defaultLogger, nil
}

func (d *defaultLogger) Close() error {
	if d.closer == nil {
		return nil
	}

	var err error
	for _, closer := range d.closer {
		if closer == nil {
			continue
		}

		if e := closer.Close(); e != nil {
			err = fmt.Errorf("%w: %q", e, err)
		}
	}

	return err
}

func (d *defaultLogger) Debug(ctx context.Context, message string, fields ...Field) {
	zapLogs := []zap.Field{
		zap.String("logType", LogTypeSYS),
		zap.String("level", "debug"),
	}

	zapLogs = append(zapLogs, formatLogs(ctx, message, d.maskEnabled, fields...)...)
	d.zapLogger.Debug(separator, zapLogs...)
}

func (d *defaultLogger) Info(ctx context.Context, message string, fields ...Field) {
	zapLogs := []zap.Field{
		zap.String("logType", LogTypeSYS),
		zap.String("level", "info"),
	}

	zapLogs = append(zapLogs, formatLogs(ctx, message, d.maskEnabled, fields...)...)
	d.zapLogger.Info(separator, zapLogs...)
}

func (d *defaultLogger) Warn(ctx context.Context, message string, fields ...Field) {
	zapLogs := []zap.Field{
		zap.String("logType", LogTypeSYS),
		zap.String("level", "warn"),
	}

	zapLogs = append(zapLogs, formatLogs(ctx, message, d.maskEnabled, fields...)...)
	d.zapLogger.Warn(separator, zapLogs...)
}

func (d *defaultLogger) Error(ctx context.Context, message string, fields ...Field) {
	zapLogs := []zap.Field{
		zap.String("logType", LogTypeSYS),
		zap.String("level", "error"),
	}

	zapLogs = append(zapLogs, formatLogs(ctx, message, d.maskEnabled, fields...)...)
	d.zapLogger.Error(separator, zapLogs...)
}

func (d *defaultLogger) Fatal(ctx context.Context, message string, fields ...Field) {
	zapLogs := []zap.Field{
		zap.String("logType", LogTypeSYS),
		zap.String("level", "fatal"),
	}

	zapLogs = append(zapLogs, formatLogs(ctx, message, d.maskEnabled, fields...)...)
	d.zapLogger.Fatal(separator, zapLogs...)
}

func (d *defaultLogger) Panic(ctx context.Context, message string, fields ...Field) {
	zapLogs := []zap.Field{
		zap.String("logType", LogTypeSYS),
		zap.String("level", "panic"),
	}

	zapLogs = append(zapLogs, formatLogs(ctx, message, d.maskEnabled, fields...)...)
	d.zapLogger.Panic(separator, zapLogs...)
}

func (d *defaultLogger) TDR(ctx context.Context, tdr LogTdrModel) {

	fields := make([]zap.Field, 0)
	fields = append(fields, zap.String("logType", LogTypeTDR))
	fields = append(fields, zap.String("level", "info"))

	// add this first, so global context value still logged
	fields = append(fields, formatLogs(ctx, separator, d.maskEnabled)...)

	fields = append(fields, zap.String("app", tdr.AppName))
	fields = append(fields, zap.String("ver", tdr.AppVersion))
	fields = append(fields, zap.String("xid", tdr.ThreadID))
	fields = append(fields, zap.String("jid", tdr.JourneyID))
	fields = append(fields, zap.String("cid", tdr.ChainID))

	fields = append(fields, zap.Any("path", tdr.Path))
	fields = append(fields, zap.String("method", tdr.Method))
	fields = append(fields, zap.Any("ip", tdr.IP))
	fields = append(fields, zap.Int("port", tdr.Port))
	fields = append(fields, zap.String("srcIP", tdr.SrcIP))
	fields = append(fields, zap.Int64("rt", tdr.RespTime))
	fields = append(fields, zap.String("rc", tdr.ResponseCode))

	fields = append(fields, formatLog("header", tdr.Header, false))
	fields = append(fields, formatLog("req", tdr.Request, d.maskEnabled))
	fields = append(fields, formatLog("resp", tdr.Response, d.maskEnabled))
	fields = append(fields, zap.String("error", tdr.Error))

	fields = append(fields, formatLog("addData", tdr.AdditionalData, d.maskEnabled))

	// exclusive: this must be write only in TDR log file
	d.zapLogger.Info(separator, fields...)
}

func formatLogs(ctx context.Context, msg string, mask bool, fields ...Field) (logRecord []zap.Field) {
	ctxVal := ExtractCtx(ctx)

	// add global value from context that must be exist on all logs!
	logRecord = append(logRecord, zap.String("message", msg))

	logRecord = append(logRecord, zap.String("_app_name", ctxVal.ServiceName))
	logRecord = append(logRecord, zap.String("_app_version", ctxVal.ServiceVersion))
	logRecord = append(logRecord, zap.Int("_app_port", ctxVal.ServicePort))
	logRecord = append(logRecord, zap.String("_app_thread_id", ctxVal.ThreadID))
	logRecord = append(logRecord, zap.String("_app_journey_id", ctxVal.JourneyID))
	logRecord = append(logRecord, zap.String("_app_chain_id", ctxVal.ChainID))
	logRecord = append(logRecord, zap.String("_app_tag", ctxVal.Tag))
	logRecord = append(logRecord, zap.String("_app_method", ctxVal.ReqMethod))
	logRecord = append(logRecord, zap.String("_app_uri", ctxVal.ReqURI))

	// add additional data that available across all log, such as user_id
	if ctxVal.AdditionalData != nil {
		logRecord = append(logRecord, zap.Any("_app_data", ctxVal.AdditionalData))
	}

	for _, field := range fields {
		logRecord = append(logRecord, formatLog(field.Key, field.Val, mask))
	}

	return
}

func formatLog(key string, msg interface{}, mask bool) (logRecord zap.Field) {
	if msg == nil {
		logRecord = zap.Any(key, struct{}{})
		return
	}

	// handle proto message
	p, ok := msg.(proto.Message)
	if ok {
		b, _err := json.Marshal(p)
		if _err != nil {
			logRecord = zap.Any(key, p.String())
			return
		}

		var data interface{}
		if _err = json.Unmarshal(b, &data); _err != nil {
			// string cannot be masked, so only try to marshal as json object
			logRecord = zap.Any(key, p.String())
			return
		}

		// use object json
		logRecord = zap.Any(key, data)
		return
	}

	// handle string, string is cannot be masked, just write it
	// but try to parse as json object if possible
	if str, ok := msg.(string); ok {
		var data interface{}
		if _err := json.Unmarshal([]byte(str), &data); _err != nil {
			logRecord = zap.String(key, str)
			return
		}

		logRecord = zap.Any(key, data)
		return
	}

	// if masking is disabled then just set as field log
	if !mask {
		logRecord = zap.Any(key, msg)
		return
	}

	// if masking is enabled and one of type supported by masking function
	switch reflect.ValueOf(msg).Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Struct:
		msgMasking := masking(msg)

		if convert, ok := msgMasking.(reflect.Value); ok {
			value := convert.Interface()
			logRecord = zap.Any(key, value)
			return
		}
	}

	// not masked since it failed to convert to reflect.Value above
	logRecord = zap.Any(key, msg)
	return
}
