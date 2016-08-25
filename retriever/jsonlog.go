package retriever

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/cloudfoundry/sonde-go/events"
	"github.com/fsnotify/fsnotify"
	"github.com/gogo/protobuf/proto"
)

const (
	OpenRetryInterval = 1 * time.Second
)

var (
	sourceInstance = "??"
)

type jsonLog struct {
	// Log is the log message
	Log string `json:"log,omitempty"`
	// Stream is the log source
	Stream string `json:"stream,omitempty"`
	// Created is the created timestamp of log
	Created time.Time `json:"time"`
}

func (j *jsonLog) Reset() {
	j.Log = ""
	j.Stream = ""
	j.Created = time.Time{}
}

func decodeLine(dec *json.Decoder, l *jsonLog) error {
	err := dec.Decode(&l)
	if err != nil {
		return err
	}
	return nil
}

type LogReader struct {
	Msg chan *events.LogMessage
	Err chan error

	source   string
	appID    string
	filename string
	file     *os.File
	tail     bool

	watcher *fsnotify.Watcher
	buf     *bytes.Buffer
}

func New(source, appID, filename string, tail bool) (*LogReader, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	watcher.Add(filename)

	r := &LogReader{
		Msg: make(chan *events.LogMessage, 4096),
		Err: make(chan error, 1),

		source:   source,
		appID:    appID,
		filename: filename,
		tail:     tail,

		watcher: watcher,
	}

	seek := os.SEEK_SET
	if r.tail {
		seek = os.SEEK_END
	}

	err = r.open(seek)
	if err != nil {
		r.Err <- err
	} else {
		go r.tailLog()
	}
	return r, nil
}

func (r *LogReader) ID() uint64 {
	fi, err := r.file.Stat()

	if err != nil {
		return 0
	}

	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return st.Ino
	}

	return 0
}

func (r *LogReader) tailLog() {
	defer func() {
		if r.watcher != nil {
			r.watcher.Close()
		}
		close(r.Msg)
	}()

	err := r.eventLoop()
	if err != nil {
		r.Err <- err
		return
	}
}

func (r *LogReader) open(seek int) error {
	fin, err := os.Open(r.filename)

	if err != nil {
		return err
	}

	// success
	fin.Seek(0, seek)
	r.file = fin
	r.watcher.Add(r.filename)
	return nil
}

func (r *LogReader) eventLoop() error {
	defer r.file.Close()
	for {
		err := r.parse()
		if !(err == nil || err == io.EOF) {
			return err
		}
		// wait events
		select {
		case event := <-r.watcher.Events:
			if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
				// watching file is removed. return for reopening.
				return nil
			}
		case err := <-r.watcher.Errors:
			return err
		}
	}
}

func (r *LogReader) restrict() error {
	stat, err := r.file.Stat()
	if err != nil {
		return err
	}
	pos, err := r.file.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}
	if stat.Size() < pos {
		// file is trancated. seek to head of file.
		_, err := r.file.Seek(0, os.SEEK_SET)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *LogReader) parse() error {
	var rdr io.Reader = r.file

	if r.buf != nil {
		rdr = io.MultiReader(r.buf, r.file)
	}

	dec := json.NewDecoder(rdr)
	log := &jsonLog{}

	for {
		err := decodeLine(dec, log)
		if err != nil {
			if err == io.EOF {
				return err
			}

			// io.ErrUnexpectedEOF is returned from json.Decoder when there is
			// remaining data in the parser's buffer while an io.EOF occurs.
			// If the json logger writes a partial json log entry to the disk
			// while at the same time the decoder tries to decode it, the race condition happens.
			if err == io.ErrUnexpectedEOF {
				if r.buf == nil {
					r.buf = &bytes.Buffer{}
				}
				r.buf.ReadFrom(dec.Buffered())
			}

			return io.EOF
		}

		r.buf = nil

		msgType := events.LogMessage_OUT
		if log.Stream == "err" {
			msgType = events.LogMessage_ERR
		}

		r.Msg <- &events.LogMessage{
			Message:        []byte(log.Log),
			AppId:          proto.String(r.appID),
			MessageType:    &msgType,
			SourceType:     &r.source,
			SourceInstance: &sourceInstance,
			Timestamp:      proto.Int64(log.Created.UnixNano()),
		}
	}
}
