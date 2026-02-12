#!/bin/bash
set -e

echo "🔧 Configuration Supabase pour PassBi Core"
echo "=========================================="

# Supabase credentials (using pooler for better compatibility)
SUPABASE_HOST="aws-1-eu-north-1.pooler.supabase.com"
SUPABASE_PORT="6543"
SUPABASE_DB="postgres"
SUPABASE_USER="postgres.xlvuggzprjjkzolonbuh"

# Prompt for password
if [ -z "$SUPABASE_PASSWORD" ]; then
    read -sp "🔐 Entrez le mot de passe Supabase: " SUPABASE_PASSWORD
    echo ""
fi

# URL encode password for connection string
ENCODED_PASSWORD=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$SUPABASE_PASSWORD'))")

# Connection string
CONN_STRING="postgresql://$SUPABASE_USER:$ENCODED_PASSWORD@$SUPABASE_HOST:$SUPABASE_PORT/$SUPABASE_DB?sslmode=require"

echo ""
echo "📡 Test de connexion à Supabase..."

# Test connection with psql
if command -v psql &> /dev/null; then
    if psql "$CONN_STRING" -c "SELECT version();" &> /dev/null; then
        echo "✅ Connexion Supabase réussie!"

        # Get PostgreSQL version
        PG_VERSION=$(psql "$CONN_STRING" -tAc "SELECT version();" | head -1)
        echo "📦 PostgreSQL: ${PG_VERSION:0:50}..."

        # Check PostGIS
        if psql "$CONN_STRING" -tAc "SELECT 1 FROM pg_extension WHERE extname = 'postgis';" | grep -q 1; then
            POSTGIS_VERSION=$(psql "$CONN_STRING" -tAc "SELECT PostGIS_Version();")
            echo "🌍 PostGIS: $POSTGIS_VERSION"
        else
            echo "⚠️  PostGIS non installé. Installation..."
            psql "$CONN_STRING" -c "CREATE EXTENSION IF NOT EXISTS postgis;"
            echo "✅ PostGIS activé!"
        fi

    else
        echo "❌ Échec de connexion à Supabase"
        echo ""
        echo "💡 Vérifications:"
        echo "   1. Votre IP est-elle autorisée dans Supabase Dashboard?"
        echo "      → https://app.supabase.com/project/xlvuggzprjjkzolonbuh/settings/database"
        echo "   2. Le mot de passe est-il correct?"
        echo "   3. Connexion SSL requise (sslmode=require)"
        exit 1
    fi
else
    echo "⚠️  psql non trouvé. Installation de postgresql-client..."
    if [[ "$OSTYPE" == "darwin"* ]]; then
        brew install postgresql
    else
        sudo apt-get install -y postgresql-client
    fi
fi

echo ""
echo "📊 Vérification des tables existantes..."
TABLE_COUNT=$(psql "$CONN_STRING" -tAc "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE';")
echo "   Tables existantes: $TABLE_COUNT"

if [ "$TABLE_COUNT" -eq "0" ]; then
    echo ""
    echo "📋 Aucune table trouvée. Application des migrations..."

    if command -v migrate &> /dev/null; then
        migrate -path migrations -database "$CONN_STRING" up
        echo "✅ Migrations appliquées!"
    else
        echo "⚠️  golang-migrate non trouvé. Installation..."
        go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
        export PATH=$PATH:$(go env GOPATH)/bin
        migrate -path migrations -database "$CONN_STRING" up
        echo "✅ Migrations appliquées!"
    fi

    # Vérifier les tables créées
    echo ""
    echo "📊 Tables créées:"
    psql "$CONN_STRING" -c "\dt"
else
    echo "✅ Tables déjà présentes"
fi

echo ""
echo "📈 Statistiques de la base de données:"
psql "$CONN_STRING" -c "
SELECT
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
"

echo ""
echo "✅ Configuration Supabase terminée!"
echo ""
echo "🔄 Prochaines étapes:"
echo "   1. Mettre à jour .env.production avec les credentials"
echo "   2. Importer les données GTFS"
echo "   3. Déployer l'API"
