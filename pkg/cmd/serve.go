package cmd

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	//"io"
	"io/ioutil"
	"log"
	"net/http"
	//"os"
	"strings"
	"time"

	"github.com/appscode/chartify/pkg/repo"
	"github.com/go-macaron/binding"
	"github.com/go-macaron/toolbox"
	"github.com/spf13/cobra"
	macaron "gopkg.in/macaron.v1"
	chartutil "k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/provenance"
	helmrepo "k8s.io/helm/pkg/repo"
)

var s string

func NewCmdServe() *cobra.Command {
	var (
		port       = 50080
		pprofPort  = 6060
		caCertFile string
		certFile   string
		keyFile    string
		pathPrefix string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve a Chart repository server",
		Run: func(cmd *cobra.Command, args []string) {
			m := macaron.New()
			m.Use(macaron.Logger())
			m.Use(macaron.Recovery())
			m.Use(toolbox.Toolboxer(m))
			m.Use(macaron.Renderer(macaron.RenderOptions{
				IndentJSON: true,
			}))
			pathPrefix = strings.Trim(pathPrefix, "/")
			if pathPrefix != "" {
				pathPrefix = "/" + pathPrefix
			}
			handler := repo.StaticBucket(repo.BucketOptions{PathPrefix: pathPrefix})
			m.Get(pathPrefix+"/:container/", handler)
			m.Get(pathPrefix+"/:container/*", handler)
			m.Post("/container", binding.MultipartForm(ChartFile{}), func(chart ChartFile) string {
				UnpackFileAndGetInfo(chart)
				return "Chart Uploaded successfully"
			})

			log.Println("Listening on port", port)

			srv := &http.Server{
				Addr:         fmt.Sprintf(":%d", port),
				ReadTimeout:  5 * time.Second,
				WriteTimeout: 10 * time.Second,
				Handler:      m,
			}
			if caCertFile == "" && certFile == "" && keyFile == "" {
				log.Fatalln(srv.ListenAndServe())
			} else {
				/*
					Ref:
					 - https://blog.cloudflare.com/exposing-go-on-the-internet/
					 - http://www.bite-code.com/2015/06/25/tls-mutual-auth-in-golang/
					 - http://www.hydrogen18.com/blog/your-own-pki-tls-golang.html
				*/
				tlsConfig := &tls.Config{
					PreferServerCipherSuites: true,
					MinVersion:               tls.VersionTLS12,
					SessionTicketsDisabled:   true,
					CipherSuites: []uint16{
						tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
						tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
						// tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, // Go 1.8 only
						// tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,   // Go 1.8 only
						tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
						tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					},
					ClientAuth: tls.VerifyClientCertIfGiven,
				}
				if caCertFile != "" {
					caCert, err := ioutil.ReadFile(caCertFile)
					if err != nil {
						log.Fatal(err)
					}
					caCertPool := x509.NewCertPool()
					caCertPool.AppendCertsFromPEM(caCert)
					tlsConfig.ClientCAs = caCertPool
				}
				tlsConfig.BuildNameToCertificate()

				srv.TLSConfig = tlsConfig
				log.Fatalln(srv.ListenAndServeTLS(certFile, keyFile))
			}
		},
	}
	cmd.Flags().IntVar(&port, "api-port", port, "Port used to serve repository")
	cmd.Flags().IntVar(&pprofPort, "pprof-port", pprofPort, "port used to run pprof tools")
	cmd.Flags().StringVar(&caCertFile, "caCertFile", caCertFile, "File containing CA certificate")
	cmd.Flags().StringVar(&certFile, "certFile", certFile, "File container server TLS certificate")
	cmd.Flags().StringVar(&keyFile, "keyFile", keyFile, "File containing server TLS private key")
	cmd.Flags().StringVar(&pathPrefix, "path-prefix", "/charts", "Path prefix for chart repositories")
	return cmd
}

func UnpackFileAndGetInfo(chart ChartFile) {
	//TODO check if the chart is empty
	file, err := chart.Data.Open()
	if err != nil {
		log.Println(err)
	}
	defer file.Close()
	c, err := chartutil.LoadArchive(file)
	if err != nil {
		log.Println(err)
	}
	hash, err := provenance.Digest(file)
	if err != nil {
		log.Println(err)
	}
	index := helmrepo.NewIndexFile()
	index.Add(c.Metadata, "test", "test-url", hash)
	//TODO Load index from bucket
	allIndex, err := helmrepo.LoadIndex([]byte(serverIndex))
	if err != nil {
		log.Println(err)
	}
	allIndex.Merge(index)
	fmt.Println(allIndex)

}