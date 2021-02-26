package yy3

import (
	"io"
	"sync"

	"github.com/yomorun/y3-codec-golang/pkg/encoding"
)

// Iterable iterate through and get the data of observe
type Iterable interface {
	Observe() <-chan interface{}
}

// Observable provide subscription and notification processing
type Observable interface {
	Iterable
	Subscribe(key byte) Observable
	OnObserve(function func(v []byte) (interface{}, error)) chan interface{}
}

type observableImpl struct {
	iterable Iterable
}

type iterableImpl struct {
	next                   chan interface{}
	subscribers            []chan interface{}
	mutex                  sync.RWMutex
	producerAlreadyCreated bool
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
	i.mutex.Lock()
	if !i.producerAlreadyCreated {
		go i.produce()
		i.producerAlreadyCreated = true
	}
	i.mutex.Unlock()
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

//FromStream reads data from reader
func FromStream(reader io.Reader) Observable {

	f := func(next chan interface{}) {
		defer close(next)
		for {
			buf := make([]byte, 3*1024)
			n, err := reader.Read(buf)

			if err != nil {
				break
			} else {
				value := buf[:n]
				next <- value
			}
		}
	}

	return createObservable(f)
}

//Processing callback function when there is data
func (o *observableImpl) OnObserve(function func(v []byte) (interface{}, error)) chan interface{} {
	_next := make(chan interface{})

	f := func(next chan interface{}) {
		defer close(next)

		observe := o.Observe()

		for {
			select {
			case item, ok := <-observe:
				if !ok {
					return
				}
				buf := item.([]byte)
				value, err := function(buf)
				if err != nil {
					return
				}

				next <- value
			}
		}
	}

	go f(_next)

	return _next
}

//Get the value of the subscribe key from the stream
func (o *observableImpl) Subscribe(key byte) Observable {

	f := func(next chan interface{}) {
		defer close(next)

		buffer := make([]byte, 0)
		var (
			index  int32  = 0    //vernier
			state  string = "RS" //RS,RLS,TS,LS,VS,REJECT
			length int32  = 0
			value  int32  = 0
			limit  int32  = 0
		)

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
						if b&0x81 == 0x81 {
							index++
							state = "RLS"
						} else {
							buffer = make([]byte, 0)
							length = 0
							value = 0
							index = 0
							limit = 0
						}
						continue

					case "RLS":
						index++
						buffer = append(buffer, b)
						l, err := decodeLength(buffer)

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
						l, err := decodeLength(buffer[1:])
						if err != nil {
							continue
						}

						length = int32(len(buffer[1:]))
						value = l
						state = "VS"
						continue
					case "VS":
						tail := int32(len(buf[i:]))
						buflength := int32(len(buffer))

						if tail >= ((1 + length + value) - buflength) {
							start := i
							end := int32(i) + (1 + length + value) - buflength
							buffer = append(buffer, buf[start:end]...)
							index += ((1 + length + value) - buflength)
							i += (int((1+length+value)-buflength) - 1)
							//check key
							k := (buffer[0] << 2) >> 2
							if k == key {
								next <- buffer
								if limit == index {
									state = "RS"
									length = 0
									value = 0
									index = 0
									limit = 0
									buffer = make([]byte, 0)
								} else {
									state = "REJECT"
								}
							} else {

								if limit == index {
									state = "RS"
									length = 0
									value = 0
									index = 0
									limit = 0
									buffer = make([]byte, 0)
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
							state = "RS"
							length = 0
							value = 0
							index = 0
							limit = 0
							buffer = make([]byte, 0)
							continue
						} else if tail >= (limit - index) {
							i += (int(limit-index) - 1)
							state = "RS"
							length = 0
							value = 0
							index = 0
							limit = 0
							buffer = make([]byte, 0)
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

func decodeLength(buf []byte) (length int32, err error) {
	varCodec := encoding.VarCodec{}
	err = varCodec.DecodePVarInt32(buf, &length)
	return
}

func createObservable(f func(next chan interface{})) Observable {
	next := make(chan interface{})
	subscribers := make([]chan interface{}, 0)

	go f(next)
	return &observableImpl{iterable: &iterableImpl{next: next, subscribers: subscribers}}
}
