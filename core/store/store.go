package store

type Store interface {
	Set(key interface{}, val interface{})
	Get(key interface{}) (interface{}, bool)
	Remove(key interface{})
	Clean()
}
