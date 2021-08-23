package decoder

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/yomorun/y3-codec-golang/pkg/common"
	"github.com/yomorun/yomo/logger"
)

const bufferSizeEnvKey = "YOMO_BUFFER_SIZE"

var (
	// bufferSize is the capacity of decoder.
	bufferSize = 200

	// dropSizeWhenFull is the size of data to drop when the buffer is full.
	dropSizeWhenFull = getDropDataSize(bufferSize)
)

// Iterable iterate through and get the data of observe
type Iterable interface {
	Observe() <-chan interface{}
}

type (
	// Marshaller defines a marshaller type (interface{} to []byte).
	Marshaller func(interface{}) ([]byte, error)
	// Unmarshaller defines an unmarshaller type ([]byte to interface).
	Unmarshaller func([]byte, interface{}) error

	// OnObserveFunc represents the callback function when the specificed key is observed.
	OnObserveFunc func(v []byte) (interface{}, error)
)

// Observable provide subscription and notification processing
type Observable interface {
	Iterable

	// Subscribe the specified key via Y3 Codec.
	Subscribe(key byte) Observable

	// OnMultiObserve calls the callback function when one of key is observed.
	OnMultiObserve(keyObserveMap map[byte]OnObserveFunc) chan KeyValue

	// OnObserve calls the callback function when the key is observed.
	OnObserve(function func(v []byte) (interface{}, error)) chan interface{}

	// MultiSubscribe gets the value of the multi keys from the stream.
	// It will return the value to next operator if any key is matched.
	MultiSubscribe(keys ...byte) Observable

	// Unmarshal transforms the items emitted by an Observable by applying an unmarshalling to each item.
	Unmarshal(unmarshaller Unmarshaller, factory func() interface{}) chan interface{}

	// RawBytes returns the raw bytes from YoMo-Zipper.
	RawBytes() chan []byte
}

type observableImpl struct {
	ctx      context.Context
	iterable Iterable
}

// KeyBuf is a pair of subscribed key and buffer.
type KeyBuf struct {
	Key byte
	Buf []byte
}

// KeyValue is a pair of observed key and value.
type KeyValue struct {
	Key   byte
	Value interface{}
}

type iterableImpl struct {
	next        chan interface{}
	subscribers []chan interface{}
	mutex       sync.RWMutex
	start       sync.Once
}

func (i *iterableImpl) Observe() <-chan interface{} {
	ch := make(chan interface{})
	i.mutex.Lock()
	i.subscribers = append(i.subscribers, ch)
	i.mutex.Unlock()
	i.connect()
	return ch
}

func (i *iterableImpl) connect() {
	i.start.Do(func() {
		go i.produce()
	})
}

func (i *iterableImpl) produce() {
	defer func() {
		i.mutex.RLock()
		for _, subscriber := range i.subscribers {
			close(subscriber)
		}
		i.mutex.RUnlock()
	}()

	for {
		select {
		case item, ok := <-i.next:
			if !ok {
				return
			}
			i.mutex.RLock()
			for _, subscriber := range i.subscribers {
				subscriber <- item
			}
			i.mutex.RUnlock()
		}
	}
}

func (o *observableImpl) Observe() <-chan interface{} {
	return o.iterable.Observe()
}

// FromItems reads data from items.
func FromItems(items []interface{}, opts ...Option) Observable {
	options := newOptions(opts...)

	f := func(ctx context.Context, next chan interface{}) {
		defer close(next)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				for _, item := range items {
					logger.Debug("[Decoder] Receive raw data from YoMo-Zipper.")
					next <- item
				}
				return
			}
		}
	}

	return createObservable(options.ctx, f)
}

// OnObserve calls the callback function when the key is observed.
func (o *observableImpl) OnObserve(function func(v []byte) (interface{}, error)) chan interface{} {
	_next := make(chan interface{})

	f := func(next chan interface{}) {
		defer close(next)

		observe := o.Observe()

		for item := range observe {
			kv := item.(KeyBuf)
			value, err := function(kv.Buf)
			if err != nil {
				// log the error and contine consuming the item from observe
				logger.Error("[Decoder] The callback function in OnObserve returns error.", "err", err)
			} else {
				next <- value
			}
		}
	}

	go f(_next)

	return _next
}

// OnMultiObserve calls the callback function when one of key is observed.
func (o *observableImpl) OnMultiObserve(keyObserveMap map[byte]OnObserveFunc) chan KeyValue {
	_next := make(chan KeyValue)

	f := func(next chan KeyValue) {
		defer close(next)

		observe := o.Observe()

		for item := range observe {
			kv := item.(KeyBuf)
			function := keyObserveMap[kv.Key]
			if function == nil {
				logger.Print("[Decoder] The OnObserve func is not found for the specified key", kv.Key)
				continue
			}
			val, err := function(kv.Buf)
			if err != nil {
				// log the error and contine consuming the item from observe
				logger.Error("[Decoder] The callback function in OnObserve returns error.", "err", err)
			} else {
				next <- KeyValue{
					Key:   kv.Key,
					Value: val,
				}
			}
		}
	}

	go f(_next)

	return _next
}

// Subscribe gets the value of the subscribe key from the stream
func (o *observableImpl) Subscribe(key byte) Observable {
	return o.MultiSubscribe(key)
}

const (
	y3StateRootStart       string = "RS"  // Root Start
	y3StateRootLengthStart string = "RLS" // Root Length Start
	y3StateTagStart        string = "TS"  // Tag Start
	y3StateLengthStart     string = "LS"  // Length Start
	y3StateValueStart      string = "VS"  // Value Start
	y3StateReject          string = "REJECT"
)

// MultiSubscribe gets the value of the multi keys from the stream.
// It will return the value to next operator if any key is matched.
//
// https://github.com/yomorun/y3-codec/blob/draft-01/draft-01.md
// 0        7
// +--------+
// | Tag    |
// +--------+--------+--------+--------+
// | Length (PVarUInt32)               |
// +--------+--------+--------+--------+
// | ...
// +--------+--------+--------+--------+
// | Value Payloads                    |
// +--------+--------+--------+--------+
// | ...
// +--------+--------+--------+--------+
func (o *observableImpl) MultiSubscribe(keys ...byte) Observable {
	// set keys to map
	m := make(map[byte]bool, len(keys))
	for _, key := range keys {
		m[key] = true
	}

	f := func(ctx context.Context, next chan interface{}) {
		defer close(next)

		buffer := make([]byte, 0)
		var (
			index          int32 // vernier
			state          = y3StateRootStart
			lengthFieldLen int32
			valueLen       int32
			limit          int32
			isPrimitive    bool
		)

		// tagLen represents the length of Tag.
		const tagLen int32 = 1

		// reset all variables
		var resetVars = func() {
			state = y3StateRootStart
			lengthFieldLen = 0
			valueLen = 0
			index = 0
			limit = 0
			buffer = make([]byte, 0)
			isPrimitive = false
		}

		// get the key of TLV packet.
		var getKey = func(b byte) byte {
			// Decoder Codec draft-1, the least significant 6 bits is the key (SeqID).
			// https://github.com/yomorun/y3-codec/blob/draft-01/draft-01.md
			return (buffer[0] << 2) >> 2
		}

		observe := o.Observe()

		for {
			select {
			case <-ctx.Done():
				return
			case item, ok := <-observe:
				if !ok {
					return
				}
				buf := item.([]byte)

				for i := 0; i < len(buf); i++ {
					b := buf[i]
					switch state {
					case y3StateRootStart:
						if common.IsRootTag(b) {
							logger.Debug("[Decoder] The first byte is a root tag, it's a node packet.", "byte", b)
							index++
							state = y3StateRootLengthStart
						} else {
							logger.Debug("[Decoder] The first byte is not a root tag, perhaps it's a primitive packet.", "byte", b)
							buffer = make([]byte, 0)
							buffer = append(buffer, b) // append tag.
							k := getKey(b)
							if !m[k] {
								logger.Debug("[Decoder] The key is not matched the observed keys.", "key", k, "observed keys", logger.BytesString(keys))
								resetVars()
								continue
							}

							// the first byte is a tag, the next state is LS (Length Start).
							state = y3StateLengthStart
							isPrimitive = true
							index++
						}
						continue

					case y3StateRootLengthStart:
						index++
						buffer = append(buffer, b)
						l, err := common.DecodeLength(buffer)

						if err != nil {
							continue
						}
						limit = index + l
						state = y3StateTagStart
						buffer = make([]byte, 0)
						continue
					case y3StateTagStart:
						index++
						buffer = make([]byte, 0)
						buffer = append(buffer, b)
						state = y3StateLengthStart
						continue
					case y3StateLengthStart:
						index++
						buffer = append(buffer, b)
						l, err := common.DecodeLength(buffer[1:])
						if err != nil {
							continue
						}

						lengthFieldLen = int32(len(buffer[1:]))
						valueLen = l
						state = y3StateValueStart
						if isPrimitive {
							limit = index + l
						}
						continue
					case y3StateValueStart:
						tail := int32(len(buf[i:]))
						bufLen := int32(len(buffer))
						// the rest length of TLV packet.
						tlvRestLen := (tagLen + lengthFieldLen + valueLen) - bufLen
						if tlvRestLen < 0 {
							logger.Error("[Decoder] The value length is greater than the length of TLV.", "value", bufLen, "tlv", tagLen+lengthFieldLen+valueLen)
							resetVars()
							continue
						}

						if tail >= tlvRestLen {
							start := i
							end := int32(i) + tlvRestLen
							// validate start index and end index.
							if start >= int(end) {
								logger.Error("[Decoder] The start index is greater than end index.", "start", start, "end", end)
								resetVars()
								continue
							}

							buffer = append(buffer, buf[start:end]...)
							index += tlvRestLen
							i += (int(tlvRestLen) - 1)
							// Decoder Codec draft-1, the least significant 6 bits is the key (SeqID).
							// https://github.com/yomorun/y3-codec/blob/draft-01/draft-01.md
							k := getKey(buffer[0])
							// check if key is matched
							if m[k] {
								// the key is matched
								// if primitive packet, it doesn't need the full TLV packet, directly returns the raw value without Tag + Length.
								if isPrimitive {
									buffer = buffer[tagLen+lengthFieldLen:]
								}

								// subscribe multi keys, return key value to distinguish the values of different keys.
								next <- KeyBuf{
									Key: k,
									Buf: buffer,
								}
								logger.Debug("[Decoder] Observe data by the specified key.", "data", logger.BytesString(buffer), "key", k)

								if limit == index {
									resetVars()
								} else {
									state = y3StateReject
								}
							} else {
								logger.Debug("[Decoder] The key is not matched the observed keys.", "key", k, "observed keys", logger.BytesString(keys))
								if limit == index {
									resetVars()
								} else {
									state = y3StateTagStart
									lengthFieldLen = 0
									valueLen = 0
								}
							}
							continue
						} else {
							buffer = append(buffer, buf[i:]...)
							index += tail
							break
						}
					case y3StateReject:
						tail := int32(len(buf[i:]))
						if limit == index {
							resetVars()
							continue
						} else if tail >= (limit - index) {
							i += (int(limit-index) - 1)
							resetVars()
							continue
						} else {
							index += tail
							break
						}
					}
				}
			}
		}
	}

	return createObservable(o.ctx, f)
}

func (o *observableImpl) Unmarshal(unmarshaller Unmarshaller, factory func() interface{}) chan interface{} {
	next := make(chan interface{})

	f := func(ctx context.Context, next chan interface{}) {
		defer close(next)

		observe := o.Observe()

		for {
			select {
			case <-ctx.Done():
				return
			case item, ok := <-observe:
				if !ok {
					return
				}

				buf := item.([]byte)
				value := factory()
				err := unmarshaller(buf, value)
				if err != nil {
					// log the error and contine consuming the item from observe
					logger.Error("[Decoder] Unmarshal error in decoder", "err", err)
				} else {
					next <- value
				}
			}
		}
	}

	go f(o.ctx, next)

	return next
}

func (o *observableImpl) RawBytes() chan []byte {
	_next := make(chan []byte)

	f := func(next chan []byte) {
		defer close(next)

		observe := o.Observe()

		for item := range observe {
			buf, ok := item.([]byte)
			if !ok {
				// log the error and contine consuming the item from observe
				logger.Error("[Decoder] The observed data is not a []byte.", "data", item)
			} else {
				next <- buf
			}
		}
	}

	go f(_next)

	return _next
}

func createObservable(ctx context.Context, f func(ctx context.Context, next chan interface{})) Observable {
	if os.Getenv(bufferSizeEnvKey) != "" {
		newSize, err := strconv.Atoi(os.Getenv(bufferSizeEnvKey))
		if newSize > 0 && err == nil {
			bufferSize = newSize
			dropSizeWhenFull = getDropDataSize(bufferSize)
		}
	}

	next := make(chan interface{}, bufferSize)
	subscribers := make([]chan interface{}, 0)

	go f(ctx, next)
	// go dropOldData(next)
	return &observableImpl{iterable: &iterableImpl{next: next, subscribers: subscribers}, ctx: ctx}
}

// dropOldData drops the old data if the size of "next" channel reaches the capacity.
func dropOldData(next chan interface{}) {
	t := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case <-t.C:
			if len(next) < bufferSize {
				// the "next" channel is not full yet.
				continue
			}

			// the "next" channel is full, drop old data to receive the new data.
			for i := 0; i < dropSizeWhenFull; i++ {
				<-next
			}
		}
	}
}

// get the drop size when the buffer is full.
func getDropDataSize(bufferSize int) int {
	dropSize := float64(bufferSize) * 0.2
	return int(dropSize)
}
