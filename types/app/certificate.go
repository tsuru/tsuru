package app

type CertificateInfo struct {
	Certificate string `json:"certificate"`
	Issuer      string `json:"issuer,omitempty"`
}

type RouterCertificateInfo struct {
	CNames map[string]CertificateInfo `json:"cnames"`
}

func (rci *RouterCertificateInfo) IsEmpty() bool {
	return len(rci.CNames) == 0
}

type CertificateSetInfo struct {
	Routers map[string]RouterCertificateInfo `json:"routers"`
}

func (csi *CertificateSetInfo) IsEmpty() bool {
	return len(csi.Routers) == 0
}
