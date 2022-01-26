package tls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/ioutil"
	"os"
)

// Create server tls config.
func CreateServerTLSConfig() (*tls.Config, error) {
	pool, err := getCACertPool()
	if err != nil {
		return nil, err
	}

	tlsCert, err := getCertAndKey()
	if err != nil {
		return nil, err
	}

	var clientAuth tls.ClientAuthType
	if isDev() {
		clientAuth = tls.NoClientCert
	} else {
		clientAuth = tls.RequireAndVerifyClientCert
	}

	return &tls.Config{
		Certificates: []tls.Certificate{*tlsCert},
		ClientCAs:    pool,
		ClientAuth:   clientAuth,
		NextProtos:   []string{"yomo"},
	}, nil
}

// Create client tls config.
func CreateClientTLSConfig() (*tls.Config, error) {
	pool, err := getCACertPool()
	if err != nil {
		return nil, err
	}

	tlsCert, err := getCertAndKey()
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		InsecureSkipVerify: isDev(),
		Certificates:       []tls.Certificate{*tlsCert},
		RootCAs:            pool,
		NextProtos:         []string{"yomo"},
		ClientSessionCache: tls.NewLRUClientSessionCache(0),
	}, nil
}

func getCACertPool() (*x509.CertPool, error) {
	var err error
	var caCert []byte

	caCertPath := os.Getenv("YOMO_TLS_CACERT_FILE")
	if len(caCertPath) == 0 {
		if isDev() {
			caCert = getDevCACert()
		}
	} else {
		caCert, err = ioutil.ReadFile(caCertPath)
		if err != nil {
			return nil, err
		}
	}

	if len(caCert) == 0 {
		return nil, errors.New("tls: cannot load CA cert")
	}

	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(caCert); !ok {
		return nil, errors.New("tls: cannot append CA cert to pool")
	}

	return pool, nil
}

func getCertAndKey() (*tls.Certificate, error) {
	var err error
	var cert, key []byte

	certPath := os.Getenv("YOMO_TLS_CERT_FILE")
	keyPath := os.Getenv("YOMO_TLS_KEY_FILE")
	if len(certPath) == 0 || len(keyPath) == 0 {
		if isDev() {
			cert, key = getDevCertAndKey()
		}
	} else {
		cert, err = ioutil.ReadFile(certPath)
		if err != nil {
			return nil, err
		}

		key, err = ioutil.ReadFile(keyPath)
		if err != nil {
			return nil, err
		}
	}

	if len(cert) == 0 || len(key) == 0 {
		return nil, errors.New("tls: cannot load tls cert/key")
	}

	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}

	return &tlsCert, nil
}

func isDev() bool {
	env := os.Getenv("YOMO_ENV")
	return len(env) == 0 || env == "development"
}

// !!! This function is for TEST only, do NOT use it in a production environment !!!
func getDevCACert() []byte {
	return []byte(`-----BEGIN CERTIFICATE-----
MIIB3TCCAWSgAwIBAgIUILqj4uyFbP+EyHVNhQ4hP/COvbMwCgYIKoZIzj0EAwIw
JjENMAsGA1UECgwEWW9NbzEVMBMGA1UEAwwMWW9NbyBSb290IENBMB4XDTIyMDEy
NjA4NTMxMVoXDTMyMDEyNDA4NTMxMVowJjENMAsGA1UECgwEWW9NbzEVMBMGA1UE
AwwMWW9NbyBSb290IENBMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEEZ2WlpfetVRE
R0sa7cC1EDN3XX3y9bY32e3573j8XfAGqxAbHvCp0kPLLzP8H4BrvcbCYEcRwdlS
ChXxfgIFJ4g5urmQozrJIxWFEsBrC/HswakEqwqCFQCPEfhlAe8po1MwUTAdBgNV
HQ4EFgQURfVDCXsj+cKrIDWfZfBJRN0JaXwwHwYDVR0jBBgwFoAURfVDCXsj+cKr
IDWfZfBJRN0JaXwwDwYDVR0TAQH/BAUwAwEB/zAKBggqhkjOPQQDAgNnADBkAjA9
qY0hbfHdba3uAjamhfhoaERh1gX8HNh09KZCOVykMyiAWwsEZo3vSE74vgA02dQC
MG/wcqBETM6Hj8u5jwIPboTWTdcybD6QdZoUYXlMLcElu9CGSBpY6Ro0aHY3xNr6
CA==
-----END CERTIFICATE-----`)
}

// !!! This function is for TEST only, do NOT use it in a production environment !!!
func getDevCertAndKey() ([]byte, []byte) {
	return []byte(`-----BEGIN CERTIFICATE-----
MIIB7zCCAXagAwIBAgIUVPVD9XjB7UIrtpLn4wPZyNHaGBMwCgYIKoZIzj0EAwIw
JjENMAsGA1UECgwEWW9NbzEVMBMGA1UEAwwMWW9NbyBSb290IENBMB4XDTIyMDEy
NjA4NTMxMVoXDTMyMDEyNDA4NTMxMVowJTENMAsGA1UECgwEWW9NbzEUMBIGA1UE
AwwLWW9NbyBTZXJ2ZXIwdjAQBgcqhkjOPQIBBgUrgQQAIgNiAAQYjP9OGw5pMCn4
djgJYOcNvu8/ptUxJzMbApyYmGK9ZA0brsNrUxbJERnWAykxB1swn/wz6JHEDLVD
ziRxYLRVS3rYqd3SRUF54FSRzsOxzXKpBWEy5tbcRt6G7L2J5jqjZjBkMCIGA1Ud
EQQbMBmCCWxvY2FsaG9zdIIMeW9tby1hcHAuZGV2MB0GA1UdDgQWBBQiQzn3YBH8
fi80WYFeGm9mLdVb9DAfBgNVHSMEGDAWgBRF9UMJeyP5wqsgNZ9l8ElE3QlpfDAK
BggqhkjOPQQDAgNnADBkAjB4Vd8HgqPWKjsTUXwq4nhetzBLq0Bgmms0ljptHoz2
1gHGWXFLc+T921FV4sryPE0CMCJOYw/lG7+kMFULxVTLjOG0Px3AMheeTXojwZ9l
ByUoaD/JYodLTsNRlJUaw0zZuQ==
-----END CERTIFICATE-----`),
		[]byte(`-----BEGIN EC PRIVATE KEY-----
MIGkAgEBBDAivM1Y5DuJHgfvbqzXFgUVnhISTgLzxEsQZTZDiO/jYijPRd9vtH00
uvklju3bLCqgBwYFK4EEACKhZANiAAQYjP9OGw5pMCn4djgJYOcNvu8/ptUxJzMb
ApyYmGK9ZA0brsNrUxbJERnWAykxB1swn/wz6JHEDLVDziRxYLRVS3rYqd3SRUF5
4FSRzsOxzXKpBWEy5tbcRt6G7L2J5jo=
-----END EC PRIVATE KEY-----`)
}
