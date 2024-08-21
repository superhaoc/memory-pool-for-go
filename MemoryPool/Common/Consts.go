package Common

const (
	MinValidAlignment        = 4
	DefaultTlsCacheAlignment = MinValidAlignment
	MaxValidAlignment        = 4096 //one page

	MaxBucketCount  = 62
	Cache_Line_Size = 64

	Partioning_Strategy_Linear           = 0
	Partioning_Strategy_Piecewise_Linear = 1
)
