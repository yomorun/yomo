package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"

	"github.com/invopop/jsonschema"
)

var (
	yomoHandler1 func(Arguments) (Result, error)                    = nil
	yomoHandler2 func(Arguments, map[string]string) (Result, error) = nil
)

type YomoRequestHeaders struct {
	Name       string `json:"name"`
	TraceID    string `json:"trace_id"`
	SpanID     string `json:"span_id"`
	BodyFormat string `json:"body_format"`
	Extension  string `json:"extension"`
}

type YomoRequestBody struct {
	Args         string `json:"args"`
	AgentContext string `json:"agent_context"`
}

type YomoResponseHeaders struct {
	StatusCode uint16 `json:"status_code"`
	ErrorMsg   string `json:"error_msg"`
	BodyFormat string `json:"body_format"`
	Extension  string `json:"extension"`
}

type YomoResponseBody struct {
	Result   any    `json:"result"`
	ErrorMsg string `json:"error_msg,omitempty"`
}

func yomoReadBytes(r io.Reader) ([]byte, error) {
	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(r, lengthBuf)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuf)

	// read the actual data
	data := make([]byte, length)
	_, err = io.ReadFull(r, data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func yomoWriteBytes(w io.Writer, data []byte) {
	// write uint32 as length of the data
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(data)))
	w.Write(lengthBuf)

	// write the actual data
	w.Write(data)
}

func yomoReadPacket[T any](r io.Reader) (*T, error) {
	buf, err := yomoReadBytes(r)
	if err != nil {
		return nil, err
	}

	// decode the packet
	var packet T
	err = json.Unmarshal(buf, &packet)
	if err != nil {
		return nil, err
	}

	return &packet, nil
}

func yomoWritePacket(w io.Writer, packet any) {
	// encode the packet
	buf, _ := json.Marshal(packet)

	yomoWriteBytes(w, buf)
}

func yomoGenerateJSONSchema() (string, error) {
	reflector := &jsonschema.Reflector{
		DoNotReference:            true,
		ExpandedStruct:            true,
		AllowAdditionalProperties: false,
	}
	parameters := reflector.Reflect(&Arguments{})

	schema := struct {
		Description string             `json:"description"`
		Parameters  *jsonschema.Schema `json:"parameters"`
	}{
		Description: Description,
		Parameters:  parameters,
	}

	buf, err := json.Marshal(schema)
	if err != nil {
		return "", err
	}

	return string(buf), nil
}

func yomoHandleStream(handlerMode int, conn io.ReadWriteCloser) error {
	defer conn.Close()

	reqHeaders, err := yomoReadPacket[YomoRequestHeaders](conn)
	if err != nil {
		yomoWritePacket(
			conn,
			&YomoResponseHeaders{
				StatusCode: 400,
				ErrorMsg:   err.Error(),
				BodyFormat: "null",
			},
		)
		return err
	}

	if reqHeaders.BodyFormat != "bytes" {
		yomoWritePacket(
			conn,
			&YomoResponseHeaders{
				StatusCode: 400,
				ErrorMsg:   "unsupported body format",
				BodyFormat: "null",
			},
		)
		return fmt.Errorf("unsupported body format: %s", reqHeaders.BodyFormat)
	}

	reqBody, err := yomoReadPacket[YomoRequestBody](conn)
	if err != nil {
		yomoWritePacket(
			conn,
			&YomoResponseHeaders{
				StatusCode: 400,
				ErrorMsg:   err.Error(),
				BodyFormat: "null",
			},
		)
		return err
	}

	var args Arguments

	if len(reqBody.Args) > 0 {
		err := json.Unmarshal([]byte(reqBody.Args), &args)
		if err != nil {
			return err
		}
	}

	resBody := &YomoResponseBody{}
	switch handlerMode {
	case 1:
		result, err := yomoHandler1(args)
		if err == nil {
			resBody.Result = result
		} else {
			resBody.ErrorMsg = err.Error()
		}
	case 2:
		var agentContext map[string]string
		if len(reqBody.AgentContext) > 0 {
			err := json.Unmarshal([]byte(reqBody.AgentContext), &agentContext)
			if err != nil {
				return err
			}
		}

		result, err := yomoHandler2(args, agentContext)
		if err == nil {
			resBody.Result = result
		} else {
			resBody.ErrorMsg = err.Error()
		}
	default:
		resBody.ErrorMsg = "unsupported handler mode"
	}

	yomoWritePacket(
		conn,
		&YomoResponseHeaders{
			StatusCode: 200,
			BodyFormat: "bytes",
		},
	)
	yomoWritePacket(conn, resBody)

	return nil
}

// reflect application Handler function
func yomoReflectApp() (int, error) {
	h := reflect.ValueOf(Handler)

	switch h.Type() {
	case reflect.TypeOf(yomoHandler1):
		reflect.ValueOf(&yomoHandler1).Elem().Set(h)
		return 1, nil
	case reflect.TypeOf(yomoHandler2):
		reflect.ValueOf(&yomoHandler2).Elem().Set(h)
		return 2, nil
	default:
		return 0, fmt.Errorf("unsupported handler type: %s", h.Type())
	}
}

func main() {
	handlerMode, err := yomoReflectApp()
	if err != nil {
		os.Exit(1)
	}

	jsonSchema, err := yomoGenerateJSONSchema()
	if err != nil {
		os.Exit(1)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("YOMO_TOOL_JSONSCHEMA: %s\n", jsonSchema)

	fmt.Printf("YOMO_TOOL_ADDR: %s\n", listener.Addr().String())

	go func() {
		for {
			io.ReadAll(os.Stdin)
			os.Exit(0)
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			os.Exit(1)
		}

		go yomoHandleStream(handlerMode, conn)
	}
}
