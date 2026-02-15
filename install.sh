#!/bin/bash

# Define cores
VERDE='\033[0;32m'
NC='\033[0m'

# Interrompe se houver erro
set -e

echo -e "${VERDE}Iniciando instalação...${NC}"

cd /home

# CORREÇÃO 1: Link do git correto (sem /tree/main)
rm -rf /home/Reverseproxy # Limpa se já existir para evitar erro
git clone https://github.com/Leonardo28113/Reverseproxy.git

# Cria diretório do Wings
WINGSDIR="/srv/wings"
mkdir -p $WINGSDIR
cd $WINGSDIR

# CORREÇÃO 2: Baixa o código fonte do Wings corretamente
echo -e "${VERDE}Baixando código fonte do Wings...${NC}"
curl -L -o wings.zip $(curl -s https://api.github.com/repos/pterodactyl/wings/releases/latest | grep zipball_url | cut -d '"' -f 4)
unzip -o wings.zip
mv pterodactyl-wings-*/* .
rm -rf pterodactyl-wings-* wings.zip

# CORREÇÃO 3: Caminhos absolutos para evitar erros de cópia
echo -e "${VERDE}Aplicando modificações...${NC}"
cp -r /home/Reverseproxy/router/* $WINGSDIR/router/
cp -f /home/Reverseproxy/router_server_proxy.go $WINGSDIR/router/

# Instalação do GO
echo -e "${VERDE}Instalando Go...${NC}"
cd /home
mkdir -p temp
cd temp

wget https://go.dev/dl/go1.22.1.linux-amd64.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf go1.22.1.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Compilação
echo -e "${VERDE}Compilando Wings (isso pode demorar)...${NC}"
cd $WINGSDIR

systemctl stop wings || true # O "|| true" impede erro se o wings não estiver rodando

# Instala dependências e compila
go get github.com/go-acme/lego/v4
go mod tidy
go build -o /usr/local/bin/wings
chmod +x /usr/local/bin/wings

# Reinicia o serviço
systemctl start wings

# Instalação do Blueprint
# CORREÇÃO 4: Corrigido erro de digitação (pterodactyl)
echo -e "${VERDE}Instalando Blueprint...${NC}"
if [ -d "/var/www/pterodactyl" ]; then
    cp -f /home/Reverseproxy/reverseproxy.blueprint /var/www/pterodactyl/
    cd /var/www/pterodactyl
    
    # Verifica se o comando blueprint existe antes de rodar
    if command -v blueprint &> /dev/null; then
        blueprint -i reverseproxy
    else
        echo "Comando 'blueprint' não encontrado. Pulei esta etapa."
    fi
else
    echo "Diretório /var/www/pterodactyl não encontrado."
fi

# Limpeza
rm -rf /home/Reverseproxy 
rm -rf /home/temp

echo -e "${VERDE}Instalado com sucesso${NC}"
