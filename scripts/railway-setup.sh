#!/bin/bash

# Script d'installation et configuration Railway pour PassBi
# Usage: ./scripts/railway-setup.sh

set -e

echo "🚂 Configuration Railway pour PassBi Core"
echo "=========================================="
echo ""

# Vérifier si Railway CLI est installé
if ! command -v railway &> /dev/null; then
    echo "❌ Railway CLI n'est pas installé"
    echo ""
    echo "Installation de Railway CLI..."
    npm install -g @railway/cli
    echo "✅ Railway CLI installé"
fi

echo ""
echo "🔐 Connexion à Railway..."
railway login --browserless

echo ""
echo "📦 Création/Liaison du projet..."
echo "Choisissez une option:"
echo "1. Créer un nouveau projet"
echo "2. Lier un projet existant"
read -p "Votre choix (1 ou 2): " choice

if [ "$choice" = "1" ]; then
    railway init
elif [ "$choice" = "2" ]; then
    railway link
else
    echo "❌ Choix invalide"
    exit 1
fi

echo ""
echo "✅ Projet configuré avec succès!"
echo ""
echo "📋 Prochaines étapes:"
echo ""
echo "1. Ajouter PostgreSQL:"
echo "   → Railway Dashboard → + New → Database → PostgreSQL"
echo ""
echo "2. Activer PostGIS:"
echo "   railway connect postgres"
echo "   CREATE EXTENSION IF NOT EXISTS postgis;"
echo ""
echo "3. Ajouter Redis:"
echo "   → Railway Dashboard → + New → Database → Redis"
echo ""
echo "4. Ajouter le service PassBi:"
echo "   → Railway Dashboard → + New → GitHub Repo → passbi_core"
echo ""
echo "5. Configurer les variables d'environnement:"
echo "   → Copier depuis .env.railway"
echo "   → Service PassBi → Variables → Ajouter les variables"
echo ""
echo "6. Déployer:"
echo "   railway up"
echo ""
echo "7. Vérifier les logs:"
echo "   railway logs"
echo ""
echo "📚 Guide complet: DEPLOY_RAILWAY.md"
echo ""
