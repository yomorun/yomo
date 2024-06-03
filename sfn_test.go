package yomo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yomorun/yomo/core"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/core/ylog"
	"github.com/yomorun/yomo/serverless"
	"github.com/yomorun/yomo/serverless/mock"
)

var (
	mockTargetString = "targetString"
)

func TestStreamFunction(t *testing.T) {
	t.Parallel()

	sfn := NewStreamFunction(
		"sfn-async-log-events",
		"localhost:9000",
		WithSfnCredential("token:<CREDENTIAL>"),
		WithSfnLogger(ylog.Default()),
		WithSfnQuicConfig(core.DefaultClientQuicConfig),
		WithSfnTLSConfig(nil),
	)
	sfn.SetObserveDataTags(0x21)

	time.AfterFunc(time.Second, func() {
		sfn.Close()
	})

	// set error handler
	sfn.SetErrorHandler(func(err error) {})

	// set handler
	sfn.SetHandler(func(ctx serverless.Context) {
		t.Logf("unittest sfn receive <- (%d)", len(ctx.Data()))
		assert.Equal(t, uint32(0x21), ctx.Tag())
		assert.Equal(t, []byte("test"), ctx.Data())

		err := ctx.WriteWithTarget(0x22, []byte("message from sfn"), mockTargetString)
		assert.Nil(t, err)

	})

	// connect to server
	err := sfn.Connect()
	assert.Nil(t, err)

	sfn.Wait()
}

func TestPipeStreamFunction(t *testing.T) {
	t.Parallel()

	sfn := NewStreamFunction("pipe-sfn", "localhost:9000", WithSfnCredential("token:<CREDENTIAL>"))
	sfn.SetObserveDataTags(0x23)

	time.AfterFunc(time.Second, func() {
		sfn.Close()
	})

	// set cron handler
	sfn.SetPipeHandler(func(in <-chan []byte, out chan<- *frame.DataFrame) {
		data := <-in
		t.Log("unittest pipe sfn receive <-", string(data))
		assert.Equal(t, "pipe test", string(data))

		out <- &frame.DataFrame{
			Tag:     0x22,
			Payload: []byte("message from pip sfn"),
		}
	})

	err := sfn.Connect()
	assert.Nil(t, err)

	sfn.Wait()
}

func TestSfnWantedTarget(t *testing.T) {
	t.Parallel()

	sfn := NewStreamFunction("sfn-handler", "localhost:9000", WithSfnCredential("token:<CREDENTIAL>"))
	sfn.SetObserveDataTags(0x22)
	sfn.SetWantedTarget(mockTargetString)

	time.AfterFunc(time.Second, func() {
		sfn.Close()
	})

	// set handler
	sfn.SetHandler(func(ctx serverless.Context) {
		t.Logf("unittest handler sfn receive <- (%d)", len(ctx.Data()))
		assert.Equal(t, uint32(0x22), ctx.Tag())
		assert.Contains(t, []string{
			"message from source",
			"message from sfn",
			"message from cron sfn",
			"message from pip sfn",
		}, string(ctx.Data()))
	})

	err := sfn.Connect()
	assert.Nil(t, err)

	sfn.Wait()
}

func TestSfnInit(t *testing.T) {
	sfn := NewStreamFunction(
		"test-sfn",
		"localhost:9000",
	)
	var total int64
	err := sfn.Init(func() error {
		total++
		return nil
	})
	assert.Nil(t, err)
	assert.Equal(t, int64(1), total)
}

func TestSfnCron(t *testing.T) {
	t.Parallel()

	sfn := NewStreamFunction("sfn-cron", "localhost:9000", WithSfnCredential("token:<CREDENTIAL>"))

	time.AfterFunc(time.Second, func() {
		sfn.Close()
	})

	// set cron handler
	sfn.SetCronHandler("@every 200ms", func(ctx serverless.CronContext) {
		t.Log("unittest cron sfn, time reached")
		ctx.Write(0x22, []byte("message from cron sfn"))
		ctx.WriteWithTarget(0x22, []byte("message from cron sfn"), mockTargetString)
	})

	err := sfn.Connect()
	assert.Nil(t, err)

	sfn.Wait()
}

func TestPatchCtxLLMFunctionCall(t *testing.T) {
	type args struct {
		serverlessCtx serverless.Context
	}
	tests := []struct {
		name        string
		args        args
		want        string
		wantWritten bool
	}{
		{
			name: "function call ok",
			args: args{
				serverlessCtx: mock.NewMockContext([]byte(`{"tid":"y8P73i-xbOr4RX2wxKd6x4URrPJLq-L1","req_id":"HHZuifDG7O41bviu","arguments":"hello yomo","tool_call_id":"call_xX0uhdsGZjPqx0hpM5JjbbjG","function_name":"say_hi","is_ok":true}`), 0x21),
			},
			want:        "",
			wantWritten: false,
		},
		{
			name: "function call not ok",
			args: args{
				serverlessCtx: mock.NewMockContext([]byte(`{"tid":"y8P73i-xbOr4RX2wxKd6x4URrPJLq-L1","req_id":"HHZuifDG7O41bviu","arguments":"","tool_call_id":"call_xX0uhdsGZjPqx0hpM5JjbbjG","function_name":"say_hi","is_ok":false}`), 0x21),
			},
			want:        `{"tid":"y8P73i-xbOr4RX2wxKd6x4URrPJLq-L1","req_id":"HHZuifDG7O41bviu","result":"this function calling do not return any message, you should ignore this.","arguments":"","tool_call_id":"call_xX0uhdsGZjPqx0hpM5JjbbjG","function_name":"say_hi","is_ok":true}`,
			wantWritten: true,
		},

		{
			name: "not a function call",
			args: args{
				serverlessCtx: mock.NewMockContext([]byte(`hello yomo`), 0x21),
			},
			want:        "",
			wantWritten: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.args.serverlessCtx
			patchCtxLLMFunctionCall(ctx)

			records := ctx.(*mock.MockContext).RecordsWritten()
			assert.Equal(t, tt.wantWritten, len(records) != 0)
			if tt.wantWritten {
				assert.Equal(t, tt.want, string(records[0].Data))
			}
		})
	}
}
