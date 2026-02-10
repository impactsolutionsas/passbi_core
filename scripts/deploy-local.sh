#!/bin/bash
set -e

echo "🚀 Déploiement Local PassBi Core"
echo "=================================="

# Check if .env exists
if [ ! -f .env ]; then
    echo "⚠️  Fichier .env non trouvé. Copie depuis .env.example..."
    cp .env.example .env
    echo "✅ Fichier .env créé. Veuillez le configurer avant de continuer."
    exit 1
fi

# Stop existing containers
echo ""
echo "🛑 Arrêt des conteneurs existants..."
docker compose down

# Build images
echo ""
echo "🔨 Build des images Docker..."
docker compose build --no-cache

# Start services
echo ""
echo "🚀 Démarrage des services..."
docker compose up -d

# Wait for services to be healthy
echo ""
echo "⏳ Attente des services..."
sleep 5

# Check health
echo ""
echo "🏥 Vérification de la santé..."
docker compose ps

# Run migrations
echo ""
echo "📊 Application des migrations..."
docker compose exec -T postgres psql -U passbi_user -d passbi << 'EOF'
\dt
EOF

echo ""
echo "✅ Déploiement local terminé!"
echo ""
echo "📍 Services disponibles:"
echo "   - API: http://localhost:8080"
echo "   - PostgreSQL: localhost:5432"
echo "   - Redis: localhost:6379"
echo ""
echo "🧪 Test rapide:"
echo "   curl http://localhost:8080/health"
echo ""
echo "📖 Import GTFS:"
echo "   docker compose exec api ./passbi-import --agency-id=dakar_dem_dikk --gtfs=/app/gtfs_folder/gtfs_Dem_Dikk.zip --rebuild-graph"
