package scraper

type Report struct {
	TargetGroup string
	Env         string
	Cluster     string
	TeamName    string
	Details     Details
	Checks      []CheckResult `json:"-"`

	maxMetricNameLen     int
	maxLabelNameLen      int
	maxLabelValueLen     int
	maxMetricCardinality int
	maxHistogramBuckets  int
	maxResponseWeight    int64
}

type CheckType string

const (
	CheckTypeMetricNameLength CheckType = "metric_name_length"
	CheckTypeLabelNameLength  CheckType = "label_name_length"
	CheckTypeLabelValueLength CheckType = "label_value_length"
	CheckTypeCardinality      CheckType = "cardinality"
	CheckTypeHistogramBuckets CheckType = "histogram_buckets"
	CheckTypeResponseWeight   CheckType = "response_weight"
)

type CheckResult struct {
	Type     CheckType `json:"-"`
	Limit    int64     `json:"-"`
	Current  int64     `json:"-"`
	Violated bool      `json:"-"`
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

	Max *MaxStats `json:"max,omitempty"`

	ResponseWeight int64 `json:"response_weight,omitempty"`
}

func (d *Details) ensureMax() {
	if d.Max == nil {
		d.Max = &MaxStats{}
	}
}

func (d *Details) addMetricNameViolation(v MetricNameViolation) {
	for i := range d.MetricNameTooLong {
		if d.MetricNameTooLong[i].MetricName == v.MetricName {
			if v.Length > d.MetricNameTooLong[i].Length {
				d.MetricNameTooLong[i] = v
			}
			d.updateMaxMetricName(v)
			return
		}
	}

	d.MetricNameTooLong = append(d.MetricNameTooLong, v)
	d.updateMaxMetricName(v)
}

func (d *Details) updateMaxMetricName(v MetricNameViolation) {
	d.ensureMax()
	if d.Max.MetricNameTooLong == nil || v.Length > d.Max.MetricNameTooLong.Length {
		d.Max.MetricNameTooLong = new(v)
	}
}

func (d *Details) addLabelNameViolation(v LabelNameViolation) {
	for i := range d.LabelNameTooLong {
		if d.LabelNameTooLong[i].MetricName == v.MetricName &&
			d.LabelNameTooLong[i].LabelName == v.LabelName {

			if v.Length > d.LabelNameTooLong[i].Length {
				d.LabelNameTooLong[i] = v
			}
			d.updateMaxLabelName(v)
			return
		}
	}

	d.LabelNameTooLong = append(d.LabelNameTooLong, v)
	d.updateMaxLabelName(v)
}

func (d *Details) updateMaxLabelName(v LabelNameViolation) {
	d.ensureMax()
	if d.Max.LabelNameTooLong == nil || v.Length > d.Max.LabelNameTooLong.Length {
		d.Max.LabelNameTooLong = new(v)
	}
}

func (d *Details) addLabelValueViolation(v LabelValueViolation) {
	for i := range d.LabelValueTooLong {
		if d.LabelValueTooLong[i].MetricName == v.MetricName &&
			d.LabelValueTooLong[i].LabelName == v.LabelName &&
			d.LabelValueTooLong[i].Value == v.Value {

			if v.Length > d.LabelValueTooLong[i].Length {
				d.LabelValueTooLong[i] = v
			}
			d.updateMaxLabelValue(v)
			return
		}
	}

	d.LabelValueTooLong = append(d.LabelValueTooLong, v)
	d.updateMaxLabelValue(v)
}

func (d *Details) updateMaxLabelValue(v LabelValueViolation) {
	d.ensureMax()
	if d.Max.LabelValueTooLong == nil || v.Length > d.Max.LabelValueTooLong.Length {
		d.Max.LabelValueTooLong = new(v)
	}
}

func (d *Details) addCardinalityViolation(v CardinalityViolation) {
	for i := range d.Cardinality {
		if d.Cardinality[i].MetricName == v.MetricName {
			if v.Value > d.Cardinality[i].Value {
				d.Cardinality[i] = v
			}
			d.updateMaxCardinality(v)
			return
		}
	}

	d.Cardinality = append(d.Cardinality, v)
	d.updateMaxCardinality(v)
}

func (d *Details) updateMaxCardinality(v CardinalityViolation) {
	d.ensureMax()
	if d.Max.Cardinality == nil || v.Value > d.Max.Cardinality.Value {
		d.Max.Cardinality = new(v)
	}
}

func (d *Details) addHistogramBucketsViolation(v HistogramBucketsViolation) {
	for i := range d.HistogramBuckets {
		if d.HistogramBuckets[i].MetricName == v.MetricName {
			if v.Buckets > d.HistogramBuckets[i].Buckets {
				d.HistogramBuckets[i] = v
			}
			d.updateMaxHistogram(v)
			return
		}
	}

	d.HistogramBuckets = append(d.HistogramBuckets, v)
	d.updateMaxHistogram(v)
}

func (d *Details) updateMaxHistogram(v HistogramBucketsViolation) {
	d.ensureMax()
	if d.Max.HistogramBuckets == nil || v.Buckets > d.Max.HistogramBuckets.Buckets {
		d.Max.HistogramBuckets = new(v)
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
