package bigquery

import (
	"fmt"
	"strings"
	"time"

	bqapi "cloud.google.com/go/bigquery"
	"github.com/pkg/errors"
)

func bqPartitioningTimeTo(t BQPartitionInfo) *bqapi.TimePartitioning {
	info := new(bqapi.TimePartitioning)
	info.Field = t.Field
	info.Expiration = time.Duration(t.Expiration) * time.Hour
	if strings.ToUpper(t.Type) == string(bqapi.HourPartitioningType) {
		info.Type = bqapi.HourPartitioningType
	} else {
		info.Type = bqapi.DayPartitioningType
	}
	return info
}

func bqPartitioningFrom(t *bqapi.TimePartitioning) *BQPartitionInfo {
	info := &BQPartitionInfo{
		Field:      t.Field,
		Type:       string(t.Type),
		Expiration: int64(t.Expiration.Hours()),
	}
	return info
}

func bqPartitioningRangeTo(t BQPartitionInfo) *bqapi.RangePartitioning {
	return &bqapi.RangePartitioning{
		Field: t.Field,
		Range: &bqapi.RangePartitioningRange{
			Start:    t.Range.Start,
			End:      t.Range.End,
			Interval: t.Range.Interval,
		},
	}
}

func bqPartitioningRangeFrom(t *bqapi.RangePartitioningRange) *BQPartitioningRange {
	return &BQPartitioningRange{
		Start:    t.Start,
		End:      t.End,
		Interval: t.Interval,
	}
}

func bqClusteringTo(ct *BQClusteringInfo) *bqapi.Clustering {
	if len(ct.Using) == 0 {
		return nil
	}
	clustering := &bqapi.Clustering{
		Fields: ct.Using,
	}
	return clustering
}

func bqClusteringFrom(ct *bqapi.Clustering) *BQClusteringInfo {
	if ct == nil || len(ct.Fields) == 0 {
		return nil
	}
	return &BQClusteringInfo{Using: ct.Fields}
}

type fieldMode struct {
	repeated bool
	required bool
}

func bqFieldModeTo(field BQField) (fieldMode, error) {
	var fm fieldMode
	if strings.ToLower(field.Mode) == "required" {
		fm.required = true
	} else if strings.ToLower(field.Mode) == "repeated" {
		fm.repeated = true
	} else if len(field.Mode) == 0 || strings.ToLower(field.Mode) == "nullable" {
		fm.required = false
	} else {
		return fm, fmt.Errorf("field %v mode should be required,repeated or nullable ", field.Name)
	}
	return fm, nil
}
func bqGoogleSheetsOptionsTo(m map[string]interface{}) *bqapi.GoogleSheetsOptions {
	var skipLeadingRows int64
	var sheetRange string

	if val, ok := m["skip_leading_rows"]; ok {
		skipLeadingRows = int64(val.(float64))
	}

	if val, ok := m["range"]; ok {
		sheetRange = val.(string)
	}

	return &bqapi.GoogleSheetsOptions{
		SkipLeadingRows: skipLeadingRows,
		Range:           sheetRange,
	}
}

func bqGoogleSheetsOptionsFrom(opt *bqapi.GoogleSheetsOptions) map[string]interface{} {
	resultMap := make(map[string]interface{})

	if opt.SkipLeadingRows != 0 {
		// Map value of int has to be converted to float because of using interface{}
		resultMap["skip_leading_rows"] = float64(opt.SkipLeadingRows)
	}
	if opt.Range != "" {
		resultMap["range"] = opt.Range
	}
	return resultMap
}

func bqExternalDataConfigTo(es BQExternalSource) (*bqapi.ExternalDataConfig, error) {
	var option bqapi.ExternalDataConfigOptions
	var sourceType bqapi.DataFormat
	switch bqapi.DataFormat(strings.ToUpper(es.SourceType)) {
	case bqapi.GoogleSheets:
		option = bqGoogleSheetsOptionsTo(es.Config)
		sourceType = bqapi.GoogleSheets
	default:
		return &bqapi.ExternalDataConfig{}, fmt.Errorf("Source format not yet implemented %s", es.SourceType)
	}

	externalConfig := &bqapi.ExternalDataConfig{
		SourceFormat: sourceType,
		SourceURIs:   es.SourceURIs,
		Options:      option,
	}
	return externalConfig, nil
}

func bqExternalDataConfigFrom(c *bqapi.ExternalDataConfig) (*BQExternalSource, error) {
	var option map[string]interface{}

	switch c.SourceFormat {
	case bqapi.GoogleSheets:
		option = bqGoogleSheetsOptionsFrom(c.Options.(*bqapi.GoogleSheetsOptions))
	default:
		return &BQExternalSource{}, fmt.Errorf("Source format not yet implemented %s", c.SourceFormat)
	}

	externalDataConfig := &BQExternalSource{
		SourceType: string(c.SourceFormat),
		SourceURIs: c.SourceURIs,
		Config:     option,
	}
	return externalDataConfig, nil
}

func bqFieldModeFrom(fm fieldMode) string {
	if fm.repeated {
		return "repeated"
	} else if fm.required {
		return "required"
	}
	return "nullable"
}

func bqSchemaTo(schema BQSchema) (bqapi.Schema, error) {
	var rv bqapi.Schema
	for _, field := range schema {
		fm, err := bqFieldModeTo(field)
		if err != nil {
			return nil, err
		}

		s := &bqapi.FieldSchema{
			Name:        field.Name,
			Type:        bqapi.FieldType(strings.ToUpper(field.Type)),
			Description: field.Description,
			Required:    fm.required,
			Repeated:    fm.repeated,
		}
		s.Schema, err = bqSchemaTo(field.Schema)
		if err != nil {
			return nil, err
		}
		rv = append(rv, s)
	}
	return rv, nil
}

func bqSchemaFrom(schema bqapi.Schema) (BQSchema, error) {
	var err error
	sc := BQSchema{}
	for _, field := range schema {
		if field == nil {
			continue
		}
		s := BQField{
			Name:        field.Name,
			Type:        string(field.Type),
			Description: field.Description,
			Mode: bqFieldModeFrom(fieldMode{
				repeated: field.Repeated,
				required: field.Required,
			}),
		}
		s.Schema, err = bqSchemaFrom(field.Schema)
		if err != nil {
			return nil, err
		}
		sc = append(sc, s)
	}
	return sc, nil
}

func bqCreateTableMetaAdapter(t BQTable) (meta *bqapi.TableMetadata, err error) {
	meta = new(bqapi.TableMetadata)
	meta.Name = t.Table
	if t.Metadata.Cluster != nil {
		meta.Clustering = bqClusteringTo(t.Metadata.Cluster)
	}
	meta.Description = t.Metadata.Description
	meta.Labels = t.Metadata.Labels

	if t.Metadata.Partition != nil {
		if t.Metadata.Partition.Range == nil {
			meta.TimePartitioning = bqPartitioningTimeTo(*t.Metadata.Partition)
		} else {
			meta.RangePartitioning = bqPartitioningRangeTo(*t.Metadata.Partition)
		}
	}

	if t.Metadata.Source != nil {
		meta.ExternalDataConfig, err = bqExternalDataConfigTo(*t.Metadata.Source)
		if err != nil {
			return nil, err
		}
	}

	if t.Metadata.ExpirationTime != "" {
		expiryTime, err := time.Parse(time.RFC3339, t.Metadata.ExpirationTime)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to parse timestamp %s", t.Metadata.ExpirationTime)
		}
		meta.ExpirationTime = expiryTime
	}
	if meta.Schema, err = bqSchemaTo(t.Metadata.Schema); err != nil {
		return nil, err
	}
	return
}

func bqUpdateTableMetaAdapter(t BQTable) (meta bqapi.TableMetadataToUpdate, err error) {
	if len(t.Table) == 0 {
		return meta, errors.New("table name cannot be empty")
	}

	meta.Name = t.Table
	if len(t.Metadata.Description) > 0 {
		meta.Description = t.Metadata.Description
	}

	if t.Metadata.Partition != nil {
		if t.Metadata.Partition.Range == nil {
			meta.TimePartitioning = bqPartitioningTimeTo(*t.Metadata.Partition)
		} else {
			// updating range based partition after creation is not supported
		}
	}
	if meta.Schema, err = bqSchemaTo(t.Metadata.Schema); err != nil {
		return
	}
	for key, value := range t.Metadata.Labels {
		meta.SetLabel(key, value)
	}

	if t.Metadata.ExpirationTime != "" {
		expiryTime, err := time.Parse(time.RFC3339, t.Metadata.ExpirationTime)
		if err != nil {
			return meta, errors.Wrapf(err, "unable to parse timestamp %s", t.Metadata.ExpirationTime)
		}
		meta.ExpirationTime = expiryTime
	}
	return
}
