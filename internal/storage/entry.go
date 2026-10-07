package storage

const (
	wheelBuckets = 60
	noWheel      = int32(-1)
)

type Entry struct {
	Value       string
	ExpiresAt   int64
	wheelBucket int32
	wheelIndex  int32
}
