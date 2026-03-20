package scraper

type LimitsConfig struct {
	MaxMetricNameLen     int   `json:"max_metric_name_len"`
	MaxLabelNameLen      int   `json:"max_label_name_len"`
	MaxLabelValueLen     int   `json:"max_label_value_len"`
	MaxMetricCardinality int   `json:"max_metric_cardinality"`
	MaxHistogramBuckets  int   `json:"max_histogram_buckets"`
	MaxBytesWeight       int64 `json:"max_bytes_weight"`
}
