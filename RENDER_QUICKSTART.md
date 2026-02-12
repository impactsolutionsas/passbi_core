# 🚀 Render Quickstart - PassBi Core

Guide ultra-rapide pour déployer PassBi sur Render en 5 minutes.

## 🎁 Pourquoi Render ?

- ✅ **100% Gratuit** pour commencer
- ✅ **Pas de carte bancaire** requise
- ✅ **PostgreSQL + Redis** inclus
- ✅ **SSL/HTTPS** automatique
- ✅ **Déploiement automatique** depuis Git
- ⚠️ Sleep après 15 min (plan gratuit)

## 🚀 Déploiement en 3 Étapes (5 minutes)

### Étape 1 : Préparer le Code (1 minute)

```bash
cd /Users/macpro/Desktop/PASSBI-DEVLAND/passbi_core

# Commit les fichiers de config Render
git add render.yaml .dockerignore Dockerfile
git commit -m "feat: add Render deployment"
git push origin main
```

### Étape 2 : Déployer sur Render (2 minutes)

1. Aller sur [dashboard.render.com](https://dashboard.render.com)
2. **Se créer un compte** (gratuit, email seulement)
3. Cliquer sur **"New +"** → **"Blueprint"**
4. **Connect GitHub** et autoriser Render
5. Sélectionner le repository **`passbi_core`**
6. Render détecte `render.yaml`
7. Cliquer sur **"Apply"**

✅ **Render crée automatiquement :**
- PostgreSQL (`passbi-postgres`)
- Redis (`passbi-redis`)
- Web Service (`passbi-api`)
- Toutes les variables d'environnement
- URL HTTPS publique

### Étape 3 : Activer PostGIS (2 minutes)

Une fois PostgreSQL créé (attendre ~1 min) :

1. Cliquer sur le service **`passbi-postgres`**
2. Onglet **"Connect"**
3. Cliquer sur **"PSQL"** (ouvre un terminal)
4. Exécuter :

```sql
CREATE EXTENSION IF NOT EXISTS postgis;
\dx  -- Vérifier
\q   -- Quitter
```

## ✅ Vérification

1. Aller sur **`passbi-api`** service
2. Attendre que le status soit **"Live"** (vert)
3. Copier l'URL : `https://passbi-api-xxxx.onrender.com`
4. Tester :

```bash
curl https://passbi-api-xxxx.onrender.com/health
```

**Réponse attendue :**

```json
{
  "status": "healthy",
  "timestamp": "2025-02-11T...",
  "checks": {
    "database": "ok",
    "redis": "ok"
  }
}
```

## 📊 Importer les Données GTFS

```bash
# Récupérer l'URL de connexion depuis Render
# Dashboard → passbi-postgres → Connect → External Database URL

export DB_HOST=<copier depuis Render>
export DB_PORT=5432
export DB_NAME=passbi
export DB_USER=passbi
export DB_PASSWORD=<copier depuis Render>
export DB_SSLMODE=require

# Lancer l'import
go run cmd/importer/main.go -gtfs-dir=./gtfs
```

## 🎯 Tester l'API

```bash
# Définir l'URL
export API_URL="https://passbi-api-xxxx.onrender.com"

# Health check
curl $API_URL/health

# Route search (Dakar)
curl "$API_URL/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467"

# Nearby stops
curl "$API_URL/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=500"

# Liste des routes
curl "$API_URL/v2/routes/list?limit=10"
```

## ⚠️ Limitation : Sleep Mode

Le plan gratuit met le service en sleep après **15 minutes** d'inactivité.

### Solution : Ping Service (Gratuit)

1. Créer un compte [UptimeRobot](https://uptimerobot.com) (gratuit)
2. Ajouter un monitor :
   - Type : **HTTP(s)**
   - URL : `https://passbi-api-xxxx.onrender.com/health`
   - Interval : **10 minutes**
3. Le service ne dormira plus jamais ! 🎉

## 💰 Coûts

**Plan Gratuit (actuel) :**
- 750h/mois (suffisant pour 1 projet)
- PostgreSQL : 1 GB
- Redis : 25 MB
- **Total : $0/mois**

**Upgrade optionnel :**
- Plan **Starter** : $7/mois
  - Pas de sleep
  - Build plus rapide
  - Plus de ressources

## 🔧 Commandes Utiles

### Voir les logs

1. Dashboard → Service `passbi-api`
2. Onglet **"Logs"**

### Se connecter à PostgreSQL

1. Service `passbi-postgres` → **"Connect"** → **"PSQL"**

### Se connecter à Redis

1. Service `passbi-redis` → **"Connect"** → **"Redis CLI"**

### Redéployer manuellement

1. Service `passbi-api` → **"Manual Deploy"** → **"Deploy latest commit"**

## 🐛 Problèmes Courants

### Build failed

**Solution :**
- Vérifier que `Dockerfile` est à la racine
- Vérifier les logs : Service → Logs → Build Logs

### Database connection failed

**Solution :**
```sql
-- Se connecter à PostgreSQL
CREATE EXTENSION IF NOT EXISTS postgis;
```

### Service en "Sleep"

**Solution :**
- Configurer UptimeRobot (voir ci-dessus)
- OU upgrade vers plan Starter ($7/mois)

### Premier appel lent (~30s)

C'est normal ! Le service se réveille après sleep.

**Solutions :**
- Attendre 30s
- Configurer UptimeRobot
- Upgrade vers plan Starter

## 📚 Documentation Complète

- **Guide détaillé** : [DEPLOY_RENDER.md](DEPLOY_RENDER.md)
- **API Documentation** : [docs/README.md](docs/README.md)
- **Render Docs** : [render.com/docs](https://render.com/docs)

## ✅ Checklist

- [ ] Code pushé sur GitHub
- [ ] Compte Render créé
- [ ] Blueprint appliqué
- [ ] PostgreSQL créé
- [ ] PostGIS activé
- [ ] Redis créé
- [ ] Web service déployé (status "Live")
- [ ] Health check OK
- [ ] Données GTFS importées
- [ ] UptimeRobot configuré (optionnel)

## 🎉 Terminé !

**Votre API :** `https://passbi-api-xxxx.onrender.com`

**Prochaines étapes :**
1. Configurer UptimeRobot pour éviter le sleep
2. Tester l'API avec les exemples
3. Intégrer dans vos applications (voir [docs/api/examples/](docs/api/examples/))

**Questions ?** Consultez [DEPLOY_RENDER.md](DEPLOY_RENDER.md) pour le guide complet.
