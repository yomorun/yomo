package decoder

import (
	"io"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/yomorun/y3-codec-golang/pkg/common"
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
	OnObserveFunc func(v []byte) (interface{}, error)
	// Marshaller defines a marshaller type (interface{} to []byte).
	Marshaller func(interface{}) ([]byte, error)
	// Unmarshaller defines an unmarshaller type ([]byte to interface).
	Unmarshaller func([]byte, interface{}) error
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
}

type observableImpl struct {
	iterable Iterable
}

// KeyObserveFunc is a pair of subscribed key and onObserve callback.
type KeyObserveFunc struct {
	Key       byte
	OnObserve OnObserveFunc
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

// FromStream reads data from reader
func FromStream(reader io.Reader) Observable {

	f := func(next chan interface{}) {
		defer close(next)
		fd := NewFrameDecoder(reader)
		for {
			// read next raw frame.
			buf, err := fd.Read(true)
			if err != nil {
				log.Printf("Receive data from zipper failed: %v", err)
				break
			} else {
				log.Printf("Receive raw data %v from zipper", buf)
				next <- buf
			}
		}
	}

	return createObservable(f)
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
				log.Println("Decoder OnObserve error:", err)
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
				log.Println("Decoder OnObserve func is not found")
				continue
			}
			val, err := function(kv.Buf)
			if err != nil {
				// log the error and contine consuming the item from observe
				log.Println("Decoder OnObserve error:", err)
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

// MultiSubscribe gets the value of the multi keys from the stream.
// It will return the value to next operator if any key is matched.
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

	f := func(next chan interface{}) {
		defer close(next)

		buffer := make([]byte, 0)
		var (
			index int32 = 0 // vernier
			// state:
			// RS: Root Start
			// RLS: Root Length Start
			// TS: Tag Start
			// LS: Length Start
			// VS: Value Start
			state       string = "RS" // RS,RLS,TS,LS,VS,REJECT
			length      int32  = 0
			value       int32  = 0
			limit       int32  = 0
			isPrimitive bool   = false
		)

		// tagLength represents the length of Tag.
		const tagLength int32 = 1

		// reset all variables
		var resetVars = func() {
			state = "RS"
			length = 0
			value = 0
			index = 0
			limit = 0
			buffer = make([]byte, 0)
			isPrimitive = false
		}

		observe := o.Observe()

		for {
			select {
			case item, ok := <-observe:
				if !ok {
					return
				}
				buf := item.([]byte)

				for i := 0; i < len(buf); i++ {
					b := buf[i]
					switch state {
					case "RS":
						if common.IsRootTag(b) {
							log.Printf("%v is a root tag, it's a node packet.", b)
							index++
							state = "RLS"
						} else {
							log.Printf("%v is not a root tag, it's a primitive packet.", b)
							// the first byte is a tag, the next state is LS (Length Start).
							state = "LS"
							isPrimitive = true
							buffer = make([]byte, 0)
							buffer = append(buffer, b) // append tag.
							index++
						}
						continue

					case "RLS":
						index++
						buffer = append(buffer, b)
						l, err := common.DecodeLength(buffer)

						if err != nil {
							continue
						}
						limit = index + l
						state = "TS"
						buffer = make([]byte, 0)
						continue
					case "TS":
						index++
						buffer = make([]byte, 0)
						buffer = append(buffer, b)
						state = "LS"
						continue
					case "LS":
						index++
						buffer = append(buffer, b)
						l, err := common.DecodeLength(buffer[1:])
						if err != nil {
							continue
						}

						length = int32(len(buffer[1:]))
						value = l
						state = "VS"
						if isPrimitive {
							limit = index + l
						}
						continue
					case "VS":
						tail := int32(len(buf[i:]))
						buflength := int32(len(buffer))

						if tail >= ((tagLength + length + value) - buflength) {
							start := i
							end := int32(i) + (tagLength + length + value) - buflength
							buffer = append(buffer, buf[start:end]...)
							index += ((tagLength + length + value) - buflength)
							i += (int((tagLength+length+value)-buflength) - 1)
							// Decoder Codec draft-1, the least significant 6 bits is the key (SeqID).
							// https://github.com/yomorun/y3-codec/blob/draft-01/draft-01.md
							k := (buffer[0] << 2) >> 2
							// check if key is matched
							if m[k] {
								// the key is matched
								// if primitive packet, it doesn't need the full TLV packet, directly returns the raw value without Tag + Length.
								if isPrimitive {
									buffer = buffer[tagLength+length:]
								}

								// subscribe multi keys, return key value to distinguish the values of different keys.
								next <- KeyBuf{
									Key: k,
									Buf: buffer,
								}

								if limit == index {
									resetVars()
									log.Println("limit == index", "RS")
								} else {
									state = "REJECT"
								}
							} else {
								log.Printf("The key %v is not matched in the observed keys %v", k, m)
								if limit == index {
									resetVars()
								} else {
									state = "TS"
									length = 0
									value = 0
								}
							}
							continue
						} else {
							buffer = append(buffer, buf[i:]...)
							index += tail
							break
						}
					case "REJECT":
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

	return createObservable(f)
}

func (o *observableImpl) Unmarshal(unmarshaller Unmarshaller, factory func() interface{}) chan interface{} {
	next := make(chan interface{})

	f := func(next chan interface{}) {
		defer close(next)

		observe := o.Observe()

		for item := range observe {
			buf := item.([]byte)
			value := factory()
			err := unmarshaller(buf, value)
			if err != nil {
				// log the error and contine consuming the item from observe
				log.Println("Decoder Unmarshal error:", err)
			} else {
				next <- value
			}
		}
	}

	go f(next)

	return next
}

func createObservable(f func(next chan interface{})) Observable {
	if os.Getenv(bufferSizeEnvKey) != "" {
		newSize, err := strconv.Atoi(os.Getenv(bufferSizeEnvKey))
		if newSize > 0 && err == nil {
			bufferSize = newSize
			dropSizeWhenFull = getDropDataSize(bufferSize)
		}
	}

	next := make(chan interface{}, bufferSize)
	subscribers := make([]chan interface{}, 0)

	go f(next)
	go dropOldData(next)
	return &observableImpl{iterable: &iterableImpl{next: next, subscribers: subscribers}}
}

// dropOldData drops the old data if the size of "next" channel reaches the capacity.
func dropOldData(next chan interface{}) {
	for {
		select {
		case <-time.After(100 * time.Millisecond):
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
