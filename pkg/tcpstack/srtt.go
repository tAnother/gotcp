package tcpstack

type SRTT struct {
	alpha  float64
	beta   float64
	minRTO float64
	maxRTO float64
	srtt   float64
	//...TBD
}

type Transmission struct {
	//...TBD
}
