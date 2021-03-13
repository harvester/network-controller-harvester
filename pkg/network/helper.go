package network

type EventSender interface {
	SendEvent(event *Event, networkType string) error
}

type Event struct {
	EventType string
	Reason    string
	Message   string
}
