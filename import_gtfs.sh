#!/bin/bash

# Load environment variables from .env
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

echo "🚆 Importing GTFS data..."
echo ""

echo "📍 Importing TER..."
./bin/passbi-import \
  --agency-id=dakar_ter \
  --gtfs=gtfs_folder/gtfs_TER.zip \
  --rebuild-graph

echo ""
echo "📍 Importing BRT..."
./bin/passbi-import \
  --agency-id=dakar_brt \
  --gtfs=gtfs_folder/gtfs_BRT.zip \
  --rebuild-graph

echo ""
echo "📍 Importing Dem Dikk..."
./bin/passbi-import \
  --agency-id=dakar_dem_dikk \
  --gtfs=gtfs_folder/gtfs_Dem_Dikk.zip \
  --rebuild-graph

echo ""
echo "📍 Importing AFTU..."
./bin/passbi-import \
  --agency-id=dakar_aftu \
  --gtfs=gtfs_folder/gtfs_AFTU.zip \
  --rebuild-graph

echo ""
echo "✅ All GTFS files imported!"
echo ""
echo "📊 Database statistics:"
/usr/local/opt/postgresql@15/bin/psql -d passbi -c "
SELECT 'Stops' as table_name, COUNT(*) as count FROM stop
UNION ALL
SELECT 'Routes', COUNT(*) FROM route
UNION ALL
SELECT 'Nodes', COUNT(*) FROM node
UNION ALL
SELECT 'Edges', COUNT(*) FROM edge;
"
