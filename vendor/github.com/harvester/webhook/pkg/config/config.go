package config

// Options for the admission webhook server
type Options struct {
	Namespace       string
	Threadiness     int
	HTTPSListenPort int

	ControllerUsername        string
	GarbageCollectionUsername string
}
