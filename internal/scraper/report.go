package scraper

type Report struct {
	TargetGroup string
	Env         string
	Cluster     string
	TeamName    string
	Details     Details
}

type MaxStats struct {
	MetricNameTooLong *MetricNameViolation       `json:"metric_name_too_long,omitempty"`
	LabelNameTooLong  *LabelNameViolation        `json:"label_name_too_long,omitempty"`
	LabelValueTooLong *LabelValueViolation       `json:"label_value_too_long,omitempty"`
	Cardinality       *CardinalityViolation      `json:"cardinality,omitempty"`
	HistogramBuckets  *HistogramBucketsViolation `json:"histogram_buckets,omitempty"`
}

type Details struct {
	MetricNameTooLong []MetricNameViolation       `json:"metric_name_too_long,omitempty"`
	LabelNameTooLong  []LabelNameViolation        `json:"label_name_too_long,omitempty"`
	LabelValueTooLong []LabelValueViolation       `json:"label_value_too_long,omitempty"`
	Cardinality       []CardinalityViolation      `json:"cardinality,omitempty"`
	HistogramBuckets  []HistogramBucketsViolation `json:"histogram_buckets,omitempty"`

	Max MaxStats `json:"max"`
}

func (d *Details) addMetricNameViolation(v MetricNameViolation) {
	d.MetricNameTooLong = append(d.MetricNameTooLong, v)

	if d.Max.MetricNameTooLong == nil || v.Length > d.Max.MetricNameTooLong.Length {
		d.Max.MetricNameTooLong = &v
	}
}

func (d *Details) addLabelNameViolation(v LabelNameViolation) {
	d.LabelNameTooLong = append(d.LabelNameTooLong, v)

	if d.Max.LabelNameTooLong == nil || v.Length > d.Max.LabelNameTooLong.Length {
		d.Max.LabelNameTooLong = &v
	}
}

func (d *Details) addLabelValueViolation(v LabelValueViolation) {
	d.LabelValueTooLong = append(d.LabelValueTooLong, v)

	if d.Max.LabelValueTooLong == nil || v.Length > d.Max.LabelValueTooLong.Length {
		d.Max.LabelValueTooLong = &v
	}
}

func (d *Details) addCardinalityViolation(v CardinalityViolation) {
	d.Cardinality = append(d.Cardinality, v)

	if d.Max.Cardinality == nil || v.Value > d.Max.Cardinality.Value {
		d.Max.Cardinality = &v
	}
}

func (d *Details) addHistogramBucketsViolation(v HistogramBucketsViolation) {
	d.HistogramBuckets = append(d.HistogramBuckets, v)

	if d.Max.HistogramBuckets == nil || v.Buckets > d.Max.HistogramBuckets.Buckets {
		d.Max.HistogramBuckets = &v
	}
}

type MetricNameViolation struct {
	MetricName string `json:"metric_name,omitempty"`
	Length     int    `json:"length,omitempty"`
}

type LabelNameViolation struct {
	MetricName string `json:"metric_name,omitempty"`
	LabelName  string `json:"label_name,omitempty"`
	Length     int    `json:"length,omitempty"`
}

type LabelValueViolation struct {
	MetricName string `json:"metric_name,omitempty"`
	LabelName  string `json:"label_name,omitempty"`
	Value      string `json:"value,omitempty"`
	Length     int    `json:"length,omitempty"`
}

type CardinalityViolation struct {
	MetricName string `json:"metric_name,omitempty"`
	Value      int    `json:"value,omitempty"`
}

type HistogramBucketsViolation struct {
	MetricName string `json:"metric_name,omitempty"`
	Buckets    int    `json:"buckets,omitempty"`
}
