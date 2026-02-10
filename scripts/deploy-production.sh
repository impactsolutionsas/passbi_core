#!/bin/bash
set -e

echo "🚀 Déploiement Production PassBi Core (Supabase)"
echo "=================================================="

# Check if .env.production exists
if [ ! -f .env.production ]; then
    echo "❌ Fichier .env.production non trouvé!"
    echo "Créez-le avec les credentials Supabase."
    exit 1
fi

# Load production env
export $(grep -v '^#' .env.production | xargs)

echo ""
echo "✅ Variables d'environnement chargées"
echo "   DB_HOST: $DB_HOST"
echo "   DB_NAME: $DB_NAME"

# Test connection to Supabase
echo ""
echo "🔍 Test de connexion Supabase..."
if ! curl -s --max-time 5 "https://$DB_HOST:$DB_PORT" > /dev/null 2>&1; then
    echo "⚠️  Impossible de se connecter à Supabase."
    echo "   Vérifiez:"
    echo "   1. Votre IP est autorisée dans Supabase Dashboard"
    echo "   2. Le mot de passe est correct"
    echo "   3. SSL est activé (DB_SSLMODE=require)"
fi

# Build production image
echo ""
echo "🔨 Build de l'image de production..."
docker build -t passbi-api:production .

# Option: Deploy to Docker Swarm / K8s / Cloud
echo ""
echo "📦 Image prête: passbi-api:production"
echo ""
echo "🚀 Options de déploiement:"
echo ""
echo "1️⃣  Docker Compose Production:"
echo "   docker-compose -f docker-compose.yml -f docker-compose.production.yml up -d"
echo ""
echo "2️⃣  Railway:"
echo "   railway up"
echo ""
echo "3️⃣  Google Cloud Run:"
echo "   gcloud run deploy passbi-api --image passbi-api:production"
echo ""
echo "4️⃣  Docker Registry Push:"
echo "   docker tag passbi-api:production registry.example.com/passbi-api:latest"
echo "   docker push registry.example.com/passbi-api:latest"
