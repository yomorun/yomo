package main

import (
	"context"
	"encoding/binary"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/serverless"
)

// ThresholdAverageValue is the threshold of the average value after a sliding window.
const ThresholdAverageValue = 13

// SlidingWindowInMS is the time in milliseconds of the sliding window.
const SlidingWindowInMS uint32 = 1e4

// SlidingTimeInMS is the interval in milliseconds of the sliding.
const SlidingTimeInMS uint32 = 1e3

// Compute avg of every past 10-seconds IoT data
var slidingAvg = func(i interface{}) error {
	values, ok := i.([]interface{})
	if ok {
		var total float32 = 0
		for _, value := range values {
			total += value.(float32)
		}
		avg := total / float32(len(values))
		log.Printf("🧩 average value in last %d ms: %f!", SlidingWindowInMS, avg)
		if avg >= ThresholdAverageValue {
			log.Printf("❗❗  average value in last %d ms: %f reaches the threshold %d!", SlidingWindowInMS, avg, ThresholdAverageValue)
		}
	}
	return nil
}

var observe = make(chan float32, 1)

func main() {
	// sfn
	sfn := yomo.NewStreamFunction(
		"Noise-3",
		"localhost:9000",
	)
	sfn.SetObserveDataTags(0x15)
	defer sfn.Close()

	sfn.SetHandler(handler)

	err := sfn.Connect()
	if err != nil {
		log.Printf("[fn3] connect err=%v", err)
		os.Exit(1)
	}

	go SlidingWindowWithTime(observe, SlidingWindowInMS, SlidingTimeInMS, slidingAvg)

	sfn.Wait()
}

func handler(ctx serverless.Context) {
	data := ctx.Data()
	v := Float32frombytes(data)
	log.Printf("✅ [fn3] observe <- %v", v)
	go func() {
		observe <- v
	}()
}

// Handler defines a function that handle the input value.
type Handler func(interface{}) error

type slidingWithTimeItem struct {
	timestamp time.Time
	data      interface{}
}

// SlidingWindowWithTime buffers the data in the specified sliding window time, the buffered data can be processed in the handler func.
// It returns the orginal data to Stream, not the buffered slice.
func SlidingWindowWithTime(observe <-chan float32, windowTimeInMS uint32, slideTimeInMS uint32, handler Handler) {
	f := func(ctx context.Context, next chan float32) {
		buf := make([]slidingWithTimeItem, 0)
		stop := make(chan struct{})
		firstTimeSend := true
		mutex := sync.Mutex{}

		checkBuffer := func() {
			mutex.Lock()
			// filter items by time
			updatedBuf := make([]slidingWithTimeItem, 0)
			availableItems := make([]interface{}, 0)
			t := time.Now().Add(-time.Duration(windowTimeInMS) * time.Millisecond)
			for _, item := range buf {
				if item.timestamp.After(t) || item.timestamp.Equal(t) {
					updatedBuf = append(updatedBuf, item)
					availableItems = append(availableItems, item.data)
				}
			}
			buf = updatedBuf

			// apply and send items
			if len(availableItems) != 0 {
				err := handler(availableItems)
				if err != nil {
					log.Printf("[fn3] SlidingWindowWithTime err=%v", err)
					return
				}
			}
			firstTimeSend = false
			mutex.Unlock()
		}

		go func() {
			defer close(next)
			for {
				select {
				case <-stop:
					checkBuffer()
					return
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(windowTimeInMS) * time.Millisecond):
					if firstTimeSend {
						checkBuffer()
					}
				case <-time.After(time.Duration(slideTimeInMS) * time.Millisecond):
					checkBuffer()
				}
			}
		}()

		for {
			select {
			case <-ctx.Done():
				close(stop)
				return
			case item, ok := <-observe:
				if !ok {
					close(stop)
					return
				}
				mutex.Lock()
				// buffer data
				buf = append(buf, slidingWithTimeItem{
					timestamp: time.Now(),
					data:      item,
				})
				mutex.Unlock()
				// immediately send the original item to downstream
				SendContext(ctx, item, next)
			}
		}
	}

	next := make(chan float32)
	go f(context.Background(), next)
}

func SendContext(ctx context.Context, input float32, ch chan<- float32) bool {
	select {
	case <-ctx.Done(): // Context's done channel has the highest priority
		return false
	default:
		select {
		case <-ctx.Done():
			return false
		case ch <- input:
			return true
		}
	}
}

func Float32frombytes(bytes []byte) float32 {
	bits := binary.BigEndian.Uint32(bytes)
	return math.Float32frombits(bits)
}
