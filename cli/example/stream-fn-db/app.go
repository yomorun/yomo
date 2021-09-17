package main

import (
	"encoding/json"
	"log"
)

// Handler will handle the raw data.
func Handler(data []byte) (byte, []byte) {
	var noise float32
	err := json.Unmarshal(data, &noise)
	if err != nil {
		log.Printf(">> [sink] unmarshal data failed, err=%v", err)
	} else {
		log.Printf(">> [sink] save `%v` to FaunaDB\n", noise)
	}

	return 0x0, nil
}

func DataID() []byte {
	return []byte { 0x34 }
}
