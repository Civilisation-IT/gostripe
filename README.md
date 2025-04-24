# GoStripe

![GoStripe Logo](https://stripe.com/img/v3/home/social.png)

GoStripe est un microservice léger et flexible pour gérer les paiements et abonnements Stripe, inspiré par l'architecture de GoTrue. Il fournit une API simple pour intégrer les fonctionnalités de paiement Stripe dans votre application.

## Fonctionnalités

- **Gestion des clients** : Création et gestion des clients Stripe
- **Sessions de paiement** : Création de sessions de paiement Stripe Checkout
- **Abonnements** : Gestion complète des abonnements
- **Webhooks** : Traitement des webhooks Stripe pour les événements de paiement
- **Base de données** : Stockage des informations clients et abonnements dans PostgreSQL
- **Migrations** : Gestion automatique des migrations de base de données
- **JWT** : Authentification via JWT pour sécuriser les endpoints

## Endpoints API

GoStripe expose les endpoints suivants :

- **POST /create-checkout-session** : Crée une session de paiement Stripe Checkout
- **POST /webhooks** : Reçoit et traite les webhooks Stripe
- **GET /get-subscription-status** : Récupère le statut d'abonnement d'un utilisateur
- **POST /cancel-subscription** : Annule un abonnement existant

## Installation

### Prérequis

- Go 1.16+
- PostgreSQL
- Compte Stripe avec clés API

### Configuration

1. Clonez le dépôt :
   ```bash
   git clone https://github.com/yourusername/gostripe.git
   cd gostripe
   ```

2. Copiez le fichier d'exemple de configuration :
   ```bash
   cp .env.example .env
   ```

3. Modifiez le fichier `.env` avec vos propres valeurs :
   ```
   # Configuration essentielle pour Stripe
   GOSTRIPE_STRIPE_SECRET_KEY=sk_test_your_stripe_secret_key
   GOSTRIPE_STRIPE_PUBLISHABLE_KEY=pk_test_your_stripe_publishable_key
   GOSTRIPE_STRIPE_WEBHOOK_SECRET=whsec_your_stripe_webhook_secret

   # Configuration de la base de données
   DATABASE_URL=postgres://username:password@localhost:5432/dbname

   # Configuration du serveur
   PORT=8082
   GOSTRIPE_API_HOST=0.0.0.0
   GOSTRIPE_LOG_LEVEL=debug

   # Configuration JWT (pour valider les tokens d'authentification)
   GOSTRIPE_JWT_SECRET=your-jwt-secret
   ```

### Compilation

```bash
go build -o gostripe
```

### Exécution

1. Exécutez les migrations de base de données :
   ```bash
   ./gostripe migrate
   ```

2. Démarrez le serveur :
   ```bash
   ./gostripe serve
   ```

## Docker

GoStripe peut être facilement déployé avec Docker :

```bash
docker build -t gostripe .
docker run -p 8082:8082 --env-file .env gostripe
```

Ou avec docker-compose :

```bash
docker-compose up -d
```

## Intégration avec Stripe

1. Créez un compte Stripe et obtenez vos clés API
2. Configurez un webhook Stripe pointant vers `https://votre-domaine.com/webhooks`
3. Ajoutez les événements suivants à votre webhook :
   - `checkout.session.completed`
   - `customer.subscription.updated`
   - `customer.subscription.deleted`

## Exemple d'utilisation

### Création d'une session de paiement

```javascript
const response = await fetch('https://votre-api.com/create-checkout-session', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer YOUR_JWT_TOKEN'
  },
  body: JSON.stringify({
    price_id: 'price_1234567890',
    success_url: 'https://votre-site.com/success',
    cancel_url: 'https://votre-site.com/cancel',
    customer_name: 'John Doe'
  })
});

const { session_id } = await response.json();

// Rediriger vers Stripe Checkout
stripe.redirectToCheckout({ sessionId: session_id });
```

### Vérification du statut d'abonnement

```javascript
const response = await fetch('https://votre-api.com/get-subscription-status', {
  method: 'GET',
  headers: {
    'Authorization': 'Bearer YOUR_JWT_TOKEN'
  }
});

const subscription = await response.json();
if (subscription.has_subscription) {
  console.log('Abonnement actif jusqu\'au', new Date(subscription.current_period_end));
} else {
  console.log('Aucun abonnement actif');
}
```

## Licence

Ce projet est sous licence GNU General Public License v3.0 - voir le fichier [LICENSE](LICENSE) pour plus de détails.

## Contributeurs

- [Votre Nom](https://github.com/yourusername)

## Remerciements

- [GoTrue](https://github.com/netlify/gotrue) pour l'inspiration architecturale
- [Stripe](https://stripe.com) pour leur excellente API de paiement