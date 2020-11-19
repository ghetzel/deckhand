package main

import (
	"net/http"

	"github.com/ghetzel/go-defaults"
	"github.com/ghetzel/go-stockutil/httputil"
	"github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/maputil"
	"github.com/ghetzel/go-stockutil/stringutil"
	"github.com/ghetzel/go-stockutil/typeutil"
)

type HttpConfig struct {
	ID          string                 `yaml:"id"`
	Method      string                 `yaml:"method" default:"get"`
	URL         string                 `yaml:"url"`
	Headers     map[string]interface{} `yaml:"headers"`
	QueryString map[string]interface{} `yaml:"params"`
	Insecure    bool                   `yaml:"insecure"`
}

func (self *HttpConfig) Key() string {
	return self.ID
}

func (self *HttpConfig) Do(page *Page) (interface{}, error) {
	defaults.SetDefaults(self)

	var url = stringutil.ExpandEnv(self.URL)

	if client, err := httputil.NewClient(url); err == nil {
		client.SetInsecureTLS(self.Insecure)

		client.SetPreRequestHook(func(req *http.Request) (interface{}, error) {
			log.Debugf("request: > %s %s", self.Method, self.URL)
			return nil, nil
		})

		client.SetPostRequestHook(func(res *http.Response, _ interface{}) error {
			log.Debugf("request: < %s [%d bytes]", res.Status, res.ContentLength)
			return nil
		})

		if res, err := client.Request(
			httputil.Method(self.Method),
			``,
			nil,
			maputil.Apply(self.QueryString, envify),
			maputil.Apply(self.Headers, envify),
		); err == nil {
			var out interface{}

			if err := client.Decode(res.Body, &out); err == nil {
				return &out, nil
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	} else {
		return nil, err
	}
}

func envify(key []string, value interface{}) (interface{}, bool) {
	return stringutil.ExpandEnv(
		typeutil.String(value),
	), true
}
