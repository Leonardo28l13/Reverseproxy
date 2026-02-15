#!/usr/bin/env bash

cd /home

git clone https://github.com/Leonardo28l13/Reverseproxy/tree/main

WINGSDIR="/srv/wings" && \
mkdir $WINGSDIR && \
cd $WINGSDIR && \
LOCATION=$(curl -s https://api.github.com/repos/pterodactyl/wings/releases/latest \
| grep "tag_name" \
| awk '{print "https://github.com/pterodactyl/wings/archive/" substr($2, 2, length($2)-3) ".zip"}') \

cd /home/Reverseproxy

cp -r router /srv/wings/wings-*

cd /home/Reverseproxy

cp -r router.go /srv/wings/wings-*/router

mkdir /home/temp

cd /home/temp

wget https://go.dev/dl/go1.22.1.linux-amd64.tar.gz && \
rm -rf /usr/local/go && \
tar -C /usr/local -xzf go1.22.1.linux-amd64.tar.gz && \
export PATH=$PATH:/usr/local/go/bin

cd /srv/wings/wings-*

systemctl stop wings && \
go get github.com/go-acme/lego/v4 && \
go mod tidy && \
go build -o /usr/local/bin/wings && \
chmod +x /usr/local/bin/wings && \
systemctl start wings

cd /home/Reverseproxy

cp -r router_server_proxy.go /srv/wings/wings-*/router

cd /home/Reverseproxy

cp -r reverseproxy.blueprint /var/www/pterodatyl

blueprint -i reverseproxy.blueprint

rm -rf /home/Reverseproxy 
rm -rf /home/temp

echo -e "\033[32mInstalado com sucesso\033[0m"
