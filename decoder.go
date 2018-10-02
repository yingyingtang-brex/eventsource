package eventsource

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"time"
)

type publication struct {
	id, event, data string
	retry           int64
}

func (s *publication) Id() string    { return s.id }
func (s *publication) Event() string { return s.event }
func (s *publication) Data() string  { return s.data }
func (s *publication) Retry() int64  { return s.retry }

// A Decoder is capable of reading Events from a stream.
type Decoder struct {
	linesCh     <-chan string
	errorCh     <-chan error
	readTimeout time.Duration
}

// NewDecoder returns a new Decoder instance that reads events
// with the given io.Reader.
func NewDecoder(r io.Reader, readTimeout time.Duration) *Decoder {
	bufReader := bufio.NewReader(newNormaliser(r))
	linesCh, errorCh := newLineStreamChannel(bufReader)
	return &Decoder{
		linesCh:     linesCh,
		errorCh:     errorCh,
		readTimeout: readTimeout,
	}
}

// Decode reads the next Event from a stream (and will block until one
// comes in).
// Graceful disconnects (between events) are indicated by an io.EOF error.
// Any error occurring mid-event is considered non-graceful and will
// show up as some other error (most likely io.ErrUnexpectedEOF).
func (dec *Decoder) Decode() (Event, error) {
	pub := new(publication)
	inDecoding := false
ReadLoop:
	for {
		var timeoutTimer *time.Timer
		var timeoutCh <-chan time.Time
		if dec.readTimeout > 0 {
			timeoutTimer = time.NewTimer(dec.readTimeout)
			timeoutCh = timeoutTimer.C
		}
		select {
		case line := <-dec.linesCh:
			if timeoutTimer != nil {
				timeoutTimer.Stop()
			}
			if line == "\n" && inDecoding {
				// the empty line signals the end of an event
				break ReadLoop
			} else if line == "\n" && !inDecoding {
				// only a newline was sent, so we don't want to publish an empty event but try to read again
				continue ReadLoop
			}
			line = strings.TrimSuffix(line, "\n")
			if strings.HasPrefix(line, ":") {
				continue ReadLoop
			}
			sections := strings.SplitN(line, ":", 2)
			field, value := sections[0], ""
			if len(sections) == 2 {
				value = strings.TrimPrefix(sections[1], " ")
			}
			inDecoding = true
			switch field {
			case "event":
				pub.event = value
			case "data":
				pub.data += value + "\n"
			case "id":
				pub.id = value
			case "retry":
				pub.retry, _ = strconv.ParseInt(value, 10, 64)
			}
		case err := <-dec.errorCh:
			if timeoutTimer != nil {
				timeoutTimer.Stop()
			}
			if err == io.ErrUnexpectedEOF && !inDecoding {
				// if we're not in the middle of an event then just return EOF
				err = io.EOF
			} else if err == io.EOF && inDecoding {
				// if we are in the middle of an event then EOF is unexpected
				err = io.ErrUnexpectedEOF
			}
			return nil, err
		case <-timeoutCh:
			return nil, ErrReadTimeout
		}
	}
	pub.data = strings.TrimSuffix(pub.data, "\n")
	return pub, nil
}

/**
 * Returns a channel that will receive lines of text as they are read. On any error
 * from the underlying reader, it stops and posts the error to a second channel.
 */
func newLineStreamChannel(r *bufio.Reader) (<-chan string, <-chan error) {
	linesCh := make(chan string)
	errorCh := make(chan error)
	go func() {
		defer close(linesCh)
		defer close(errorCh)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				errorCh <- err
				return
			}
			linesCh <- line
		}
	}()
	return linesCh, errorCh
}
