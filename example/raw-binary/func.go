package main

import (
	"log"

	"github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/rx"
)

func Handler(rx rx.RxStream) rx.RxStream {
	return rx.Subscribe(0x10).OnObserve(f).Encode(0x11)
}

var f = func(v []byte) (interface{}, error) {
	log.Printf("f.v=%v", v)
	p, _, _, _ := y3.DecodePrimitivePacket(v)
	if p != nil {
		log.Printf("raw data=%v", p.ToBytes())
	}

	return "test", nil
}
