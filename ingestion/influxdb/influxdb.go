package influxdb

import (
	"context"
	"reflect"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/mehrdadrad/tcpdog/config"
	"github.com/mehrdadrad/tcpdog/geo"
	pb "github.com/mehrdadrad/tcpdog/proto"
)

const maxChanSize = 1000

type influxdb struct {
	geo           geo.Geoer
	cfg           *dbConfig
	serialization string
}

// Start starts ingestion data points to influxdb
func Start(ctx context.Context, name string, ser string, ch chan interface{}) error {
	var g geo.Geoer

	cfg := config.FromContextServer(ctx)
	iCfg := influxDBConfig(cfg.Ingestion[name].Config)

	opts, err := influxdbOpts(iCfg)
	if err != nil {
		return err
	}

	client := influxdb2.NewClientWithOptions(iCfg.URL, iCfg.Token, opts)
	writeAPI := client.WriteAPI(iCfg.Org, iCfg.Bucket)

	// if geo is available
	if v, ok := geo.Reg[cfg.Geo.Type]; ok {
		g = v
		g.Init(cfg.Logger(), cfg.Geo.Config)
	}

	i := influxdb{geo: g, cfg: iCfg, serialization: ser}

	pCh := make(chan *write.Point, maxChanSize)

	for c := uint(0); c < iCfg.Workers; c++ {
		go i.pWorker(ctx, ch, pCh)
	}

	// main influxdb loop
	go func() {
		for {
			select {
			case p := <-pCh:
				writeAPI.WritePoint(p)
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// pWorker creates influxdb point
func (i *influxdb) pWorker(ctx context.Context, ch chan interface{}, pCh chan *write.Point) {
	var fields interface{}

	point := i.getPointMaker(i.serialization)

	for {
		select {
		case fields = <-ch:
			p := point(fields)
			if p != nil {
				pCh <- p
			}
		case <-ctx.Done():
			return
		}
	}
}

func (i *influxdb) getPointMaker(ser string) func(fi interface{}) *write.Point {
	switch ser {
	case "json":
		return i.pointJSON
	case "spb":
		return i.pointSPB
	case "pb":
		return i.pointPB
	}

	return nil
}

// pointSPB returns influxdb pointSPB with geo (if available)
func (i *influxdb) pointSPB(fi interface{}) *write.Point {
	var (
		tags      = map[string]string{}
		fields    = map[string]interface{}{}
		timestamp time.Time
	)

	f := fi.(*pb.FieldsSPB)

	for key, field := range f.Fields.Fields {
		if value, ok := field.GetKind().(*structpb.Value_StringValue); ok {
			if i.geo != nil && (key == i.cfg.GeoField) {
				for k1, v1 := range i.geo.Get(value.StringValue) {
					tags[k1] = v1
				}
				continue
			}
			tags[key] = value.StringValue
		} else if key != "Timestamp" {
			fields[key] = field.GetNumberValue()
		} else {
			timestamp = time.Unix(int64(field.GetNumberValue()), 0)
		}
	}

	return influxdb2.NewPoint("tcpdog", tags, fields, timestamp)
}

// point returns influxdb point with geo (if available)
func (i *influxdb) pointPB(fi interface{}) *write.Point {
	var (
		tags      = map[string]string{}
		fields    = map[string]interface{}{}
		timestamp time.Time
	)

	v := reflect.ValueOf(fi.(*pb.Fields)).Elem()

	for n := 3; n < v.NumField(); n++ {
		if v.Field(n).Pointer() != 0 {
			switch v.Field(n).Addr().Elem().Elem().Kind() {
			case reflect.String:
				if i.geo != nil && (v.Type().Field(n).Name == i.cfg.GeoField) {
					for k1, v1 := range i.geo.Get(v.Field(n).Elem().String()) {
						tags[k1] = v1
					}
					continue
				}
				tags[v.Type().Field(n).Name] = v.Field(n).Elem().String()
			case reflect.Uint32:
				fields[v.Type().Field(n).Name] = v.Field(n).Elem().Uint()
			case reflect.Uint64:
				if v.Type().Field(n).Name != "Timestamp" {
					fields[v.Type().Field(n).Name] = v.Field(n).Elem().Uint()
				} else {
					timestamp = time.Unix(int64(v.Field(n).Elem().Uint()), 0)
				}
			}
		}
	}

	return influxdb2.NewPoint("tcpdog", tags, fields, timestamp)
}

func (i *influxdb) pointJSON(fi interface{}) *write.Point {
	var (
		tags      = map[string]string{}
		fields    = map[string]interface{}{}
		timestamp time.Time
	)

	f := fi.(map[string]interface{})

	for key, field := range f {
		if value, ok := field.(string); ok {
			if i.geo != nil && (key == i.cfg.GeoField) {
				for k1, v1 := range i.geo.Get(value) {
					tags[k1] = v1
				}
				continue
			}
			tags[key] = value
		} else if key != "Timestamp" {
			fields[key] = field.(float64)
		} else {
			timestamp = time.Unix(int64(field.(float64)), 0)
		}
	}

	return influxdb2.NewPoint("tcpdog", tags, fields, timestamp)
}

// influxdbOpts returns influxdb options
func influxdbOpts(cfg *dbConfig) (*influxdb2.Options, error) {
	opts := influxdb2.DefaultOptions()
	opts.SetMaxRetries(cfg.MaxRetries)
	opts.SetHTTPRequestTimeout(cfg.Timeout)
	opts.SetBatchSize(cfg.BatchSize)

	// TLS
	if cfg.TLSConfig.Enable {
		tlsConfig, err := config.GetTLS(&cfg.TLSConfig)
		if err != nil {
			return nil, err
		}
		opts = opts.SetTLSConfig(tlsConfig)
	}

	return opts, nil
}
