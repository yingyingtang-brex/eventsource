package eventsource

import (
	"bytes"
	"sync"
)

type cachedEncodingEvent struct {
	wrapped Event
	once    *sync.Once
	encoded []byte
}

func makeCachedEncodingEvent(evt Event) cachedEncodingEvent {
	return cachedEncodingEvent{
		wrapped: evt,
		once:    new(sync.Once),
		encoded: nil,
	}
}

func (evt cachedEncodingEvent) Encode() []byte {
	evt.once.Do(func() {
		var buff bytes.Buffer
		enc := NewEncoder(&buff, false)
		enc.Encode(evt)
		evt.encoded = buff.Bytes()
	})

	return evt.encoded
}

func (evt cachedEncodingEvent) Id() string {
	return evt.wrapped.Id()
}

func (evt cachedEncodingEvent) Event() string {
	return evt.wrapped.Event()
}

func (evt cachedEncodingEvent) Data() string {
	return evt.wrapped.Data()
}
