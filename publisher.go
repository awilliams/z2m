package z2m

import "path"

type Publisher interface {
	Publish(topic string, payload []byte) error
}

func PublisherFunc(f func(topic string, payload []byte) error) Publisher {
	return publisherFunc(f)
}

type publisherFunc func(topic string, payload []byte) error

func (p publisherFunc) Publish(topic string, payload []byte) error {
	return p(topic, payload)
}

func PrefixPublisher(prefix string, p Publisher) Publisher {
	return PublisherFunc(func(topic string, payload []byte) error {
		topic = path.Join(prefix, topic)
		return p.Publish(topic, payload)
	})
}
