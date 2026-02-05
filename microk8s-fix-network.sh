#!/bin/bash
# MicroK8s Network Fix Script
# Regeneriert die Zertifikate mit der aktuellen IP-Adresse

set -e

echo "=== MicroK8s Network Fix Script ==="
echo ""

# MicroK8s stoppen
echo "Stoppe MicroK8s..."
sudo microk8s stop
sleep 5

# Zertifikate neu generieren
echo "Regeneriere Zertifikate mit aktueller IP..."
sudo microk8s refresh-certs --cert front-proxy-client.crt
sudo microk8s refresh-certs --cert ca.crt

# MicroK8s starten
echo "Starte MicroK8s..."
sudo microk8s start
sleep 10

# Warte bis MicroK8s bereit ist
echo "Warte auf MicroK8s..."
microk8s status --wait-ready

echo ""
echo "=== Fix abgeschlossen ==="
echo ""
echo "Teste die Verbindung:"
microk8s kubectl get nodes -o wide
