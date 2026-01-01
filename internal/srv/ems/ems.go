package ems

type EMS struct {
	URL string
}

func NewEMS(url string) *EMS {
	return &EMS{URL: url}
}

func (e *EMS) GetURL() string {
	return e.URL
}
