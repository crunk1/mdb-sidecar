package main

import (
	"fmt"
	"os"
	"reflect"
	"strconv"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	var err error
	fakePod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: cfg.PodName}}
	cfg.podOrdinal, err = podOrd(fakePod)
	if err != nil {
		panic(err)
	}
	cfg.podFQDN = podFQDN(fakePod)
	cfg.podFQDNAndPort = podFQDNAndPort(fakePod)

	fmt.Printf("Config: %+v\n", cfg)
}
