Note: we will be downloading and modifying our files in /srv/wings, if it doesn't exist, please create it first with mkdir /srv/wings

----------------------------------------------------------------------------------------

Run "
WINGSDIR="/srv/wings" && \
mkdir $WINGSDIR && \
cd $WINGSDIR && \
LOCATION=$(curl -s https://api.github.com/repos/pterodactyl/wings/releases/latest \
| grep "tag_name" \
| awk '{print "https://github.com/pterodactyl/wings/archive/" substr($2, 2, length($2)-3) ".zip"}') \
; curl -L -o wings_latest.zip $LOCATION && \
unzip wings_latest.zip
"
Then enter the folder where the files are unzipped, most likely wings-<whatever the latest version is>

----------------------------------------------------------------------------------------

Upload the contents of wings_installation

----------------------------------------------------------------------------------------

Paste this line in the file router/router.go after server.POST("/ws/deny", postServerDenyWSTokens)

		server.POST("/proxy/create", postServerProxyCreate)
		server.POST("/proxy/delete", postServerProxyDelete)

----------------------------------------------------------------------------------------

Install GoLang using the following command:
wget https://go.dev/dl/go1.22.1.linux-amd64.tar.gz && \
rm -rf /usr/local/go && \
tar -C /usr/local -xzf go1.22.1.linux-amd64.tar.gz && \
export PATH=$PATH:/usr/local/go/bin

To verify your installation, run "go version"

----------------------------------------------------------------------------------------

Build the new wings instance using the following command:
systemctl stop wings && \
go get github.com/go-acme/lego/v4 && \
go mod tidy && \
go build -o /usr/local/bin/wings && \
chmod +x /usr/local/bin/wings && \
systemctl start wings

This may take a while to finish, please be patient.

----------------------------------------------------------------------------------------

Make sure you have the latest version of Nginx installed on all of your nodes.

Don't forget to create the folder /srv/server_certs for storing the SSL certificates.
You may change this location if you know what you are doing.
