package websocketnats

// InputMessage input message entity
type InputMessage struct {
	InputTime  int64  `json:"inputTime"`
	UserID     string `json:"userId"`
	DeviceID   string `json:"deviceId"`
	Host       string `json:"host"`
	RemoteAddr string `json:"remoteAddr"`
	Body       []byte `json:"data"`
}
