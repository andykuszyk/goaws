package app

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	log "github.com/sirupsen/logrus"
)

type SqsErrorType struct {
	HttpError int
	Type      string
	Code      string
	Message   string
}

func (s *SqsErrorType) Error() string {
	return s.Type
}

var SqsErrors map[string]SqsErrorType

type Message struct {
	MessageBody            []byte
	Uuid                   string
	MD5OfMessageAttributes string
	MD5OfMessageBody       string
	ReceiptHandle          string
	ReceiptTime            time.Time
	VisibilityTimeout      time.Time
	NumberOfReceives       int
	Retry                  int
	MessageAttributes      map[string]MessageAttributeValue
	GroupID                string
	SentTime			   time.Time
}

func (m *Message) IsReadyForReceipt() bool {
	randomLatency, err := getRandomLatency()
	if err != nil {
		log.Error(err)
		return true
	}
	return m.SentTime.Add(randomLatency).Before(time.Now())
}

func getRandomLatency() (time.Duration, error){
	minVar := os.Getenv("GOAWS_RANDOM_LATENCY_MIN")
	maxVar := os.Getenv("GOAWS_RANDOM_LATENCY_MAX")
	if minVar == "" || maxVar == "" {
		return time.Duration(0), nil
	}
	min, err := strconv.Atoi(minVar)
	if err != nil {
		return time.Duration(0), errors.New(fmt.Sprintf("Invalid value for GOAWS_RANDOM_LATENCY_MIN: %s", minVar))
	}
	max, err := strconv.Atoi(maxVar)
	if err != nil {
		return time.Duration(0), errors.New(fmt.Sprintf("Invalid value for GOAWS_RANDOM_LATENCY_MAX: %s", maxVar))
	}
	var randomLatencyValue int
	if max == min {
		randomLatencyValue = max
	} else {
		randomLatencyValue = rand.Intn(max-min) + min
	}
	randomDuration, err := time.ParseDuration(fmt.Sprintf("%dms", randomLatencyValue))
	if err != nil {
		return time.Duration(0), errors.New(fmt.Sprintf("Error parsing random latency value: %dms", randomLatencyValue))
	}
	return randomDuration, nil
}

type MessageAttributeValue struct {
	Name     string
	DataType string
	Value    string
	ValueKey string
}

type Queue struct {
	Name                string
	URL                 string
	Arn                 string
	TimeoutSecs         int
	ReceiveWaitTimeSecs int
	Messages            []Message
	DeadLetterQueue     *Queue
	MaxReceiveCount     int
	IsFIFO              bool
	FIFOMessages        map[string]int
	FIFOSequenceNumbers map[string]int
}

var SyncQueues = struct {
	sync.RWMutex
	Queues map[string]*Queue
}{Queues: make(map[string]*Queue)}

func HasFIFOQueueName(queueName string) bool {
	return strings.HasSuffix(queueName, ".fifo")
}

func (q *Queue) NextSequenceNumber(groupId string) string {
	if _, ok := q.FIFOSequenceNumbers[groupId]; !ok {
		q.FIFOSequenceNumbers = map[string]int{
			groupId: 0,
		}
	}

	q.FIFOSequenceNumbers[groupId]++
	return strconv.Itoa(q.FIFOSequenceNumbers[groupId])
}

func (q *Queue) IsLocked(groupId string) bool {
	_, ok := q.FIFOMessages[groupId]
	return ok
}

func (q *Queue) LockGroup(groupId string) {
	if _, ok := q.FIFOMessages[groupId]; !ok {
		q.FIFOMessages = map[string]int{
			groupId: 0,
		}
	}
}

func (q *Queue) UnlockGroup(groupId string) {
	if _, ok := q.FIFOMessages[groupId]; ok {
		delete(q.FIFOMessages, groupId)
	}
}
