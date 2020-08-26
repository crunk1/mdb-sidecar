package main

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
)

var cfg struct {
	PodName string `env:"POD_NAME"`
	NS      string `env:"NS"`
	RSSvc   string `env:"RS_SVC"`
	MDBUser string `env:"MDB_USER"`
	MDBPass string `env:"MDB_PASS"`
	MDBPort uint16 `env:"MDB_PORT"`

	// Helper fields derived from env vars.
	podFQDN        string
	podFQDNAndPort string
	podOrdinal     uint8
}

func init() {
	cv := reflect.ValueOf(&cfg).Elem()
	ct := cv.Type()

	for i := 0; i < ct.NumField(); i++ {
		fv := cv.Field(i)
		ft := ct.Field(i)

		if !fv.CanSet() {
			continue
		}

		envvarName, _ := ft.Tag.Lookup("env")
		v := os.Getenv(envvarName)
		if v == "" {
			continue
		}

		switch k := fv.Kind(); k {
		case reflect.String:
			fv.SetString(v)
		case reflect.Uint16:
			vu, err := strconv.ParseUint(v, 10, 16)
			if err != nil {
				panic(err)
			}
			fv.SetUint(vu)
		default:
			panic("unsupported cfg field Kind: " + k.String())
		}
	}

	ord, err := strconv.ParseUint(cfg.PodName[len(cfg.RSSvc)+1:], 10, 8)
	if err != nil {
		panic(err)
	}
	cfg.podOrdinal = uint8(ord)
	cfg.podFQDN = fmt.Sprintf("%s.%s.%s.svc.cluster.local", cfg.PodName, cfg.RSSvc, cfg.NS)
	cfg.podFQDNAndPort = cfg.podFQDN + ":" + strconv.FormatUint(uint64(cfg.MDBPort), 10)

	fmt.Printf("Config: %+v\n", cfg)
}
