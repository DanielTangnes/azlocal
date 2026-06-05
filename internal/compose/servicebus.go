package compose

import (
	"encoding/json"

	"github.com/DanielTangnes/azlocal/internal/config"
)

// defaultSBNamespace is the namespace the Service Bus emulator uses when the
// config doesn't specify one. It is internal to the emulator; clients connect
// with UseDevelopmentEmulator=true regardless of this value.
const defaultSBNamespace = "sbemulatorns"

// Service Bus emulator entity defaults, matching the documented Config.json
// example. The emulator rejects entities whose Properties block is incomplete,
// so every field is emitted explicitly.
const (
	sbDefaultTTL       = "PT1H"
	sbDefaultLock      = "PT1M"
	sbDefaultDupWindow = "PT20S"
	sbDefaultMaxDeliv  = 3
)

// sbConfig mirrors the Service Bus emulator Config.json schema.
type sbConfig struct {
	UserConfig sbUserConfig `json:"UserConfig"`
}

type sbUserConfig struct {
	Namespaces []sbNamespace `json:"Namespaces"`
	Logging    sbLogging     `json:"Logging"`
}

type sbLogging struct {
	Type string `json:"Type"`
}

type sbNamespace struct {
	Name   string    `json:"Name"`
	Queues []sbQueue `json:"Queues"`
	Topics []sbTopic `json:"Topics"`
}

type sbQueue struct {
	Name       string       `json:"Name"`
	Properties sbQueueProps `json:"Properties"`
}

type sbTopic struct {
	Name          string           `json:"Name"`
	Properties    sbTopicProps     `json:"Properties"`
	Subscriptions []sbSubscription `json:"Subscriptions"`
}

type sbSubscription struct {
	Name       string     `json:"Name"`
	Properties sbSubProps `json:"Properties"`
}

type sbQueueProps struct {
	DeadLetteringOnMessageExpiration    bool   `json:"DeadLetteringOnMessageExpiration"`
	DefaultMessageTimeToLive            string `json:"DefaultMessageTimeToLive"`
	DuplicateDetectionHistoryTimeWindow string `json:"DuplicateDetectionHistoryTimeWindow"`
	ForwardDeadLetteredMessagesTo       string `json:"ForwardDeadLetteredMessagesTo"`
	ForwardTo                           string `json:"ForwardTo"`
	LockDuration                        string `json:"LockDuration"`
	MaxDeliveryCount                    int    `json:"MaxDeliveryCount"`
	RequiresDuplicateDetection          bool   `json:"RequiresDuplicateDetection"`
	RequiresSession                     bool   `json:"RequiresSession"`
}

type sbTopicProps struct {
	DefaultMessageTimeToLive            string `json:"DefaultMessageTimeToLive"`
	DuplicateDetectionHistoryTimeWindow string `json:"DuplicateDetectionHistoryTimeWindow"`
	RequiresDuplicateDetection          bool   `json:"RequiresDuplicateDetection"`
}

type sbSubProps struct {
	DeadLetteringOnMessageExpiration bool   `json:"DeadLetteringOnMessageExpiration"`
	DefaultMessageTimeToLive         string `json:"DefaultMessageTimeToLive"`
	LockDuration                     string `json:"LockDuration"`
	MaxDeliveryCount                 int    `json:"MaxDeliveryCount"`
	ForwardDeadLetteredMessagesTo    string `json:"ForwardDeadLetteredMessagesTo"`
	ForwardTo                        string `json:"ForwardTo"`
	RequiresSession                  bool   `json:"RequiresSession"`
}

func defaultQueueProps() sbQueueProps {
	return sbQueueProps{
		DefaultMessageTimeToLive:            sbDefaultTTL,
		DuplicateDetectionHistoryTimeWindow: sbDefaultDupWindow,
		LockDuration:                        sbDefaultLock,
		MaxDeliveryCount:                    sbDefaultMaxDeliv,
	}
}

func defaultTopicProps() sbTopicProps {
	return sbTopicProps{
		DefaultMessageTimeToLive:            sbDefaultTTL,
		DuplicateDetectionHistoryTimeWindow: sbDefaultDupWindow,
	}
}

func defaultSubProps() sbSubProps {
	return sbSubProps{
		DefaultMessageTimeToLive: sbDefaultTTL,
		LockDuration:             sbDefaultLock,
		MaxDeliveryCount:         sbDefaultMaxDeliv,
	}
}

// namespaceOr returns the configured namespace, or the emulator default.
func namespaceOr(sb *config.ServiceBusService) string {
	if sb != nil && sb.Namespace != "" {
		return sb.Namespace
	}
	return defaultSBNamespace
}

// GenerateServiceBusConfig renders the emulator Config.json for the declared
// queues, topics, and subscriptions.
func GenerateServiceBusConfig(sb *config.ServiceBusService) ([]byte, error) {
	// Initialize to empty (non-nil) slices so absent sections serialize as []
	// rather than null, which the emulator can reject.
	ns := sbNamespace{Name: namespaceOr(sb), Queues: []sbQueue{}, Topics: []sbTopic{}}
	for _, q := range sb.Queues {
		ns.Queues = append(ns.Queues, sbQueue{Name: q, Properties: defaultQueueProps()})
	}
	for _, t := range sb.Topics {
		topic := sbTopic{Name: t.Name, Properties: defaultTopicProps(), Subscriptions: []sbSubscription{}}
		for _, s := range t.Subscriptions {
			topic.Subscriptions = append(topic.Subscriptions, sbSubscription{
				Name:       s,
				Properties: defaultSubProps(),
			})
		}
		ns.Topics = append(ns.Topics, topic)
	}

	cfg := sbConfig{UserConfig: sbUserConfig{
		Namespaces: []sbNamespace{ns},
		Logging:    sbLogging{Type: "Console"},
	}}
	return json.MarshalIndent(cfg, "", "  ")
}
