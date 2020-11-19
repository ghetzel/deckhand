package main

type LiteralConfig struct {
	ID    string      `yaml:"id"`
	Value interface{} `yaml:"value"`
}

func (self *LiteralConfig) Key() string {
	return self.ID
}

func (self *LiteralConfig) Do(page *Page) (interface{}, error) {
	return self.Value, nil
}
