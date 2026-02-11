#!/bin/bash
set -e

echo "📦 Import GTFS vers Supabase"
echo "============================"

# Check if .env.production exists
if [ ! -f .env.production ]; then
    echo "❌ Fichier .env.production non trouvé!"
    echo ""
    echo "Création du fichier .env.production..."
    cat > .env.production << 'EOF'
# Production Database (Supabase)
DB_HOST=db.xlvuggzprjjkzolonbuh.supabase.co
DB_PORT=5432
DB_NAME=postgres
DB_USER=postgres
DB_PASSWORD=your_password_here
DB_SSLMODE=require

# Redis (Upstash or cloud)
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# API Configuration
API_PORT=8080
CACHE_TTL=10m
MAX_EXPLORED_NODES=50000
ROUTE_TIMEOUT=10s
EOF
    echo "✅ Fichier .env.production créé"
    echo "⚠️  Veuillez éditer .env.production et configurer DB_PASSWORD"
    echo ""
    read -p "Appuyez sur Entrée après avoir configuré le fichier..."
fi

# Load production environment
export $(grep -v '^#' .env.production | xargs)

echo ""
echo "🔍 Configuration chargée:"
echo "   DB_HOST: $DB_HOST"
echo "   DB_NAME: $DB_NAME"
echo "   DB_USER: $DB_USER"

# Check GTFS files
echo ""
echo "📂 Fichiers GTFS disponibles:"
if [ -d "gtfs_folder" ]; then
    ls -lh gtfs_folder/*.zip 2>/dev/null || echo "   Aucun fichier GTFS trouvé"
else
    echo "   ❌ Dossier gtfs_folder non trouvé"
    exit 1
fi

# Ask which agencies to import
echo ""
echo "🚌 Agences à importer:"
echo "   1. Dakar Dem Dikk (DDD) - 2.5 MB - ~53 routes"
echo "   2. AFTU - 10 MB - ~73 routes"
echo "   3. BRT - 508 KB - ~2 routes"
echo "   4. TER - 107 KB - ~6 routes"
echo "   5. Toutes les agences"
echo ""
read -p "Choisissez (1-5): " CHOICE

case $CHOICE in
    1)
        AGENCIES=("dakar_dem_dikk:gtfs_folder/gtfs_Dem_Dikk.zip")
        REBUILD_LAST="--rebuild-graph"
        ;;
    2)
        AGENCIES=("dakar_aftu:gtfs_folder/gtfs_AFTU.zip")
        REBUILD_LAST="--rebuild-graph"
        ;;
    3)
        AGENCIES=("dakar_brt:gtfs_folder/gtfs_BRT.zip")
        REBUILD_LAST="--rebuild-graph"
        ;;
    4)
        AGENCIES=("dakar_ter:gtfs_folder/gtfs_TER.zip")
        REBUILD_LAST="--rebuild-graph"
        ;;
    5)
        AGENCIES=(
            "dakar_dem_dikk:gtfs_folder/gtfs_Dem_Dikk.zip"
            "dakar_aftu:gtfs_folder/gtfs_AFTU.zip"
            "dakar_brt:gtfs_folder/gtfs_BRT.zip"
            "dakar_ter:gtfs_folder/gtfs_TER.zip"
        )
        REBUILD_LAST="--rebuild-graph"
        ;;
    *)
        echo "❌ Choix invalide"
        exit 1
        ;;
esac

# Import agencies
for i in "${!AGENCIES[@]}"; do
    AGENCY_DATA="${AGENCIES[$i]}"
    AGENCY_ID="${AGENCY_DATA%%:*}"
    GTFS_FILE="${AGENCY_DATA##*:}"

    # Check if it's the last agency
    if [ $i -eq $((${#AGENCIES[@]} - 1)) ]; then
        REBUILD="$REBUILD_LAST"
    else
        REBUILD=""
    fi

    echo ""
    echo "📥 Import: $AGENCY_ID"
    echo "   Fichier: $GTFS_FILE"
    echo "   Rebuild: ${REBUILD:-non}"

    START_TIME=$(date +%s)

    go run cmd/importer/main.go \
        --agency-id="$AGENCY_ID" \
        --gtfs="$GTFS_FILE" \
        $REBUILD

    END_TIME=$(date +%s)
    DURATION=$((END_TIME - START_TIME))

    echo "✅ Import terminé en ${DURATION}s"
done

echo ""
echo "🎉 Import GTFS terminé!"
echo ""
echo "📊 Vérification des données importées..."

# Connection string for verification
CONN_STRING="postgresql://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=$DB_SSLMODE"

# Verify data
echo ""
psql "$CONN_STRING" -c "
SELECT
  'Stops' as type, COUNT(*)::text as count FROM stop
UNION ALL
SELECT 'Routes', COUNT(*)::text FROM route
UNION ALL
SELECT 'Nodes', COUNT(*)::text FROM node
UNION ALL
SELECT 'Edges', COUNT(*)::text FROM edge
UNION ALL
SELECT 'Agencies', COUNT(DISTINCT agency_id)::text FROM route;
"

echo ""
echo "✅ Import des données GTFS terminé avec succès!"
echo ""
echo "🚀 Prochaine étape: Déployer l'API"
echo "   ./scripts/deploy-production.sh"
