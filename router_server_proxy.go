package router

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

type LetsEncryptUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *LetsEncryptUser) GetEmail() string {
	return u.Email
}
func (u LetsEncryptUser) GetRegistration() *registration.Resource {
	return u.Registration
}
func (u *LetsEncryptUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

// postServerProxyCreate cria o proxy do servidor e configura o Nginx,
// validando o domínio e realizando a solicitação do certificado com Let's Encrypt.
func postServerProxyCreate(c *gin.Context) {
	s := ExtractServer(c)

	var data struct {
		Domain         string `json:"domain"`
		IP             string `json:"ip"` // Pode ser um IP literal ou um hostname (ex.: bots.stelarcloud.com)
		Port           string `json:"port"`
		Ssl            bool   `json:"ssl"`
		UseLetsEncrypt bool   `json:"use_lets_encrypt"`
		ClientEmail    string `json:"client_email"`
		SslCert        string `json:"ssl_cert"`
		SslKey         string `json:"ssl_key"`
	}

	// Validação dos dados de entrada
	if err := c.BindJSON(&data); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "Erro ao ler os dados da requisição.",
		})
		return
	}

	// Validação: o domínio deve estar em letras minúsculas
	if data.Domain != strings.ToLower(data.Domain) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "O domínio deve estar em letras minúsculas.",
		})
		return
	}

	// Validação DNS:
	// Caso o campo de alocação (data.IP) seja um hostname, resolve-o para obter os IPs.
	var allocationIPs []string
	if ip := net.ParseIP(data.IP); ip != nil {
		allocationIPs = []string{data.IP}
	} else {
		var err error
		allocationIPs, err = net.LookupHost(data.IP)
		if err != nil || len(allocationIPs) == 0 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Falha ao resolver o host de alocação '%s'.", data.IP),
			})
			return
		}
	}

	// Resolve o domínio do proxy para obter seus IPs
	domainIPs, err := net.LookupHost(data.Domain)
	if err != nil || len(domainIPs) == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Falha ao resolver o domínio '%s'.", data.Domain),
		})
		return
	}

	// Verifica se algum dos IPs do domínio está entre os IPs da alocação
	matched := false
	for _, dip := range domainIPs {
		for _, aip := range allocationIPs {
			if dip == aip {
				matched = true
				break
			}
		}
		if matched {
			break
		}
	}
	if !matched {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("O domínio %s não está apontado para nenhum dos IPs %v do host de alocação %s. Verifique seu registro DNS.", data.Domain, allocationIPs, data.IP),
		})
		return
	}

	// Configuração inicial do Nginx (HTTP)
	nginxconfig := []byte(`server {
	listen 80;
	server_name ` + data.Domain + `;

	location / {
		proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
		proxy_set_header Host $http_host;
		proxy_pass http://` + data.IP + `:` + data.Port + `;
	}

	location /.well-known/acme-challenge/ {
		proxy_set_header Host $host;
		proxy_pass http://127.0.0.1:81$request_uri;
	}
}`)

	nginxConfigPath := "/etc/nginx/sites-available/" + data.Domain + "_" + data.Port + ".conf"
	if err := os.WriteFile(nginxConfigPath, nginxconfig, 0644); err != nil {
		s.Log().WithField("error", err).Error("Falha ao escrever a configuração do Nginx em " + nginxConfigPath)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "Erro interno ao salvar a configuração do servidor proxy.",
		})
		return
	}

	lncmd := exec.Command(
		"ln",
		"-s",
		nginxConfigPath,
		"/etc/nginx/sites-enabled/"+data.Domain+"_"+data.Port+".conf",
	)
	if err := lncmd.Run(); err != nil {
		s.Log().WithField("error", err).Error("Falha ao criar link simbólico para " + nginxConfigPath)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "Erro interno ao configurar o proxy.",
		})
		return
	}

	restartcmd := exec.Command("systemctl", "reload", "nginx")
	if err := restartcmd.Run(); err != nil {
		s.Log().WithField("error", err).Error("Falha ao reiniciar o Nginx.")
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "Erro interno ao reiniciar o servidor web.",
		})
		return
	}

	var certfile []byte
	var keyfile []byte

	certPath := "/srv/server_certs/" + data.Domain + "/cert.pem"
	keyPath := "/srv/server_certs/" + data.Domain + "/key.pem"

	if data.Ssl {
		if data.UseLetsEncrypt {
			// Gerar chave privada para solicitação do certificado
			privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				s.Log().WithField("error", err).Error("Falha ao gerar chave privada para Let's Encrypt")
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "Erro interno ao gerar chave privada para Let's Encrypt.",
				})
				return
			}

			letsEncryptUser := LetsEncryptUser{
				Email: data.ClientEmail,
				key:   privateKey,
			}

			config := lego.NewConfig(&letsEncryptUser)
			config.Certificate.KeyType = certcrypto.RSA2048

			client, err := lego.NewClient(config)
			if err != nil {
				s.Log().WithField("error", err).Error("Falha ao criar o cliente do Let's Encrypt")
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"error": "Erro ao criar o cliente do Let's Encrypt. Verifique o email e as configurações.",
				})
				return
			}

			err = client.Challenge.SetHTTP01Provider(http01.NewProviderServer("", "81"))
			if err != nil {
				s.Log().WithField("error", err).Error("Falha ao configurar o provedor HTTP01 para Let's Encrypt")
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"error": "Erro ao configurar o provedor HTTP01. Verifique se a porta 81 está liberada.",
				})
				return
			}

			reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
			if err != nil {
				s.Log().WithField("error", err).Error("Falha ao registrar a conta no Let's Encrypt")
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"error": "Erro ao registrar a conta no Let's Encrypt. Verifique seu email e aceite os termos de serviço.",
				})
				return
			}
			letsEncryptUser.Registration = reg

			request := certificate.ObtainRequest{
				Domains: []string{data.Domain},
				Bundle:  true,
			}

			cert, err := client.Certificate.Obtain(request)
			if err != nil {
				s.Log().WithField("error", err).Error("Falha ao obter o certificado para " + data.Domain)
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Erro ao obter o certificado para %s. Certifique-se de que o domínio está corretamente apontado e que a porta 81 está acessível.", data.Domain),
				})
				return
			}

			certfile = []byte(cert.Certificate)
			keyfile = []byte(cert.PrivateKey)
		} else {
			certfile = []byte(data.SslCert)
			keyfile = []byte(data.SslKey)
		}

		// Cria os diretórios para salvar o certificado e a chave
		if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
			s.Log().WithField("error", err).Error("Falha ao criar diretório para o certificado em " + filepath.Dir(certPath))
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Erro interno ao salvar o certificado.",
			})
			return
		}
		if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
			s.Log().WithField("error", err).Error("Falha ao criar diretório para a chave em " + filepath.Dir(keyPath))
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Erro interno ao salvar o certificado.",
			})
			return
		}

		if err := os.WriteFile(certPath, certfile, 0644); err != nil {
			s.Log().WithField("error", err).Error("Falha ao escrever o certificado em " + certPath)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Erro ao salvar o certificado.",
			})
			return
		}

		if err := os.WriteFile(keyPath, keyfile, 0644); err != nil {
			s.Log().WithField("error", err).Error("Falha ao escrever a chave em " + keyPath)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Erro ao salvar a chave do certificado.",
			})
			return
		}

		// Atualiza a configuração do Nginx para redirecionar para HTTPS
		nginxconfig = []byte(`server {
	listen 80;
	server_name ` + data.Domain + `;
	return 301 https://$server_name$request_uri;
}

server {
	listen 443 ssl http2;
	server_name ` + data.Domain + `;

	ssl_certificate ` + certPath + `;
	ssl_certificate_key ` + keyPath + `;
	ssl_session_cache shared:SSL:10m;
	ssl_protocols TLSv1.2 TLSv1.3;
	ssl_ciphers "ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384";
	ssl_prefer_server_ciphers on;

	location / {
		proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
		proxy_set_header Host $http_host;
		proxy_pass http://` + data.IP + `:` + data.Port + `;
	}

	location /.well-known/acme-challenge/ {
		proxy_set_header Host $host;
		proxy_pass http://127.0.0.1:81$request_uri;
	}
}`)
		if err := os.WriteFile(nginxConfigPath, nginxconfig, 0644); err != nil {
			s.Log().WithField("error", err).Error("Falha ao atualizar a configuração do Nginx para HTTPS em " + nginxConfigPath)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Erro interno ao atualizar a configuração do proxy.",
			})
			return
		}
		restartcmd = exec.Command("systemctl", "reload", "nginx")
		if err := restartcmd.Run(); err != nil {
			s.Log().WithField("error", err).Error("Falha ao reiniciar o Nginx após atualizar para HTTPS.")
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Erro interno ao reiniciar o servidor web.",
			})
			return
		}
	}

	c.Status(http.StatusAccepted)
}

// postServerProxyDelete remove as configurações do proxy e reinicia o Nginx.
func postServerProxyDelete(c *gin.Context) {
	s := ExtractServer(c)

	var data struct {
		Domain string `json:"domain"`
		Port   string `json:"port"`
	}

	if err := c.BindJSON(&data); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "Erro ao ler os dados da requisição.",
		})
		return
	}

	configPath := "/etc/nginx/sites-available/" + data.Domain + "_" + data.Port + ".conf"
	if err := os.RemoveAll(configPath); err != nil {
		s.Log().WithField("error", err).Error("Falha ao remover a configuração do Nginx em " + configPath)
	}

	enabledPath := "/etc/nginx/sites-enabled/" + data.Domain + "_" + data.Port + ".conf"
	if err := os.RemoveAll(enabledPath); err != nil {
		s.Log().WithField("error", err).Error("Falha ao remover a configuração do Nginx em " + enabledPath)
	}

	cmd := exec.Command("systemctl", "reload", "nginx")
	if err := cmd.Run(); err != nil {
		s.Log().WithField("error", err).Error("Falha ao reiniciar o Nginx após remover o proxy.")
	}

	c.Status(http.StatusAccepted)
}