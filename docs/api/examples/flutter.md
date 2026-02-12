# Exemples Flutter

Intégration complète de PassBi API dans une application Flutter.

## Installation

Ajoutez les dépendances dans `pubspec.yaml` :

```yaml
dependencies:
  flutter:
    sdk: flutter
  http: ^1.1.0
  provider: ^6.1.1
  cached_network_image: ^3.3.0
```

## Modèles de Données

```dart
// models/coordinates.dart
class Coordinates {
  final double lat;
  final double lon;

  Coordinates({required this.lat, required this.lon});

  @override
  String toString() => '$lat,$lon';
}

// models/route_result.dart
class RouteResult {
  final int durationSeconds;
  final int walkDistanceMeters;
  final int transfers;
  final List<Step> steps;

  RouteResult({
    required this.durationSeconds,
    required this.walkDistanceMeters,
    required this.transfers,
    required this.steps,
  });

  factory RouteResult.fromJson(Map<String, dynamic> json) {
    return RouteResult(
      durationSeconds: json['duration_seconds'],
      walkDistanceMeters: json['walk_distance_meters'],
      transfers: json['transfers'],
      steps: (json['steps'] as List)
          .map((step) => Step.fromJson(step))
          .toList(),
    );
  }

  int get durationMinutes => (durationSeconds / 60).floor();
}

// models/step.dart
enum StepType { walk, ride, transfer }

class Step {
  final StepType type;
  final String fromStop;
  final String toStop;
  final String fromStopName;
  final String toStopName;
  final String? route;
  final String? routeName;
  final String? mode;
  final int durationSeconds;
  final int? distanceMeters;
  final int? numStops;

  Step({
    required this.type,
    required this.fromStop,
    required this.toStop,
    required this.fromStopName,
    required this.toStopName,
    this.route,
    this.routeName,
    this.mode,
    required this.durationSeconds,
    this.distanceMeters,
    this.numStops,
  });

  factory Step.fromJson(Map<String, dynamic> json) {
    return Step(
      type: _parseStepType(json['type']),
      fromStop: json['from_stop'],
      toStop: json['to_stop'],
      fromStopName: json['from_stop_name'],
      toStopName: json['to_stop_name'],
      route: json['route'],
      routeName: json['route_name'],
      mode: json['mode'],
      durationSeconds: json['duration_seconds'],
      distanceMeters: json['distance_meters'],
      numStops: json['num_stops'],
    );
  }

  static StepType _parseStepType(String type) {
    switch (type) {
      case 'WALK':
        return StepType.walk;
      case 'RIDE':
        return StepType.ride;
      case 'TRANSFER':
        return StepType.transfer;
      default:
        throw Exception('Unknown step type: $type');
    }
  }

  int get durationMinutes => (durationSeconds / 60).floor();
}
```

## Service API

```dart
// services/passbi_service.dart
import 'dart:convert';
import 'package:http/http.dart' as http;
import '../models/route_result.dart';
import '../models/coordinates.dart';

class PassBiService {
  final String baseUrl;
  final Duration timeout;

  PassBiService({
    this.baseUrl = 'http://localhost:8080',
    this.timeout = const Duration(seconds: 15),
  });

  Future<Map<String, RouteResult>> searchRoutes(
    Coordinates from,
    Coordinates to,
  ) async {
    final uri = Uri.parse('$baseUrl/v2/route-search').replace(
      queryParameters: {
        'from': from.toString(),
        'to': to.toString(),
      },
    );

    try {
      final response = await http.get(uri).timeout(timeout);

      if (response.statusCode == 200) {
        final data = json.decode(response.body);
        final routes = <String, RouteResult>{};

        (data['routes'] as Map<String, dynamic>).forEach((key, value) {
          routes[key] = RouteResult.fromJson(value);
        });

        return routes;
      } else if (response.statusCode == 404) {
        throw PassBiException('Aucun itinéraire trouvé entre ces locations');
      } else {
        final error = json.decode(response.body);
        throw PassBiException(error['error'] ?? 'Erreur inconnue');
      }
    } catch (e) {
      if (e is PassBiException) rethrow;
      throw PassBiException('Erreur de connexion: $e');
    }
  }

  Future<List<NearbyStop>> findNearbyStops(
    double lat,
    double lon, {
    int radius = 500,
  }) async {
    final uri = Uri.parse('$baseUrl/v2/stops/nearby').replace(
      queryParameters: {
        'lat': lat.toString(),
        'lon': lon.toString(),
        'radius': radius.toString(),
      },
    );

    final response = await http.get(uri).timeout(timeout);

    if (response.statusCode == 200) {
      final data = json.decode(response.body);
      return (data['stops'] as List)
          .map((stop) => NearbyStop.fromJson(stop))
          .toList();
    } else {
      throw PassBiException('Erreur lors de la recherche d\'arrêts');
    }
  }
}

class PassBiException implements Exception {
  final String message;
  PassBiException(this.message);

  @override
  String toString() => message;
}

class NearbyStop {
  final String id;
  final String name;
  final double lat;
  final double lon;
  final int distanceMeters;
  final List<String> routes;
  final int routesCount;

  NearbyStop({
    required this.id,
    required this.name,
    required this.lat,
    required this.lon,
    required this.distanceMeters,
    required this.routes,
    required this.routesCount,
  });

  factory NearbyStop.fromJson(Map<String, dynamic> json) {
    return NearbyStop(
      id: json['id'],
      name: json['name'],
      lat: json['lat'],
      lon: json['lon'],
      distanceMeters: json['distance_meters'],
      routes: List<String>.from(json['routes']),
      routesCount: json['routes_count'],
    );
  }
}
```

## Provider pour l'État

```dart
// providers/route_provider.dart
import 'package:flutter/foundation.dart';
import '../services/passbi_service.dart';
import '../models/route_result.dart';
import '../models/coordinates.dart';

class RouteProvider with ChangeNotifier {
  final PassBiService _service;

  RouteProvider(this._service);

  Coordinates? _from;
  Coordinates? _to;
  Map<String, RouteResult>? _routes;
  bool _loading = false;
  String? _error;

  Coordinates? get from => _from;
  Coordinates? get to => _to;
  Map<String, RouteResult>? get routes => _routes;
  bool get loading => _loading;
  String? get error => _error;

  void setFrom(Coordinates coords) {
    _from = coords;
    notifyListeners();
  }

  void setTo(Coordinates coords) {
    _to = coords;
    notifyListeners();
  }

  Future<void> searchRoutes() async {
    if (_from == null || _to == null) return;

    _loading = true;
    _error = null;
    notifyListeners();

    try {
      _routes = await _service.searchRoutes(_from!, _to!);
    } catch (e) {
      _error = e.toString();
    } finally {
      _loading = false;
      notifyListeners();
    }
  }
}
```

## Widget de Carte d'Itinéraire

```dart
// widgets/route_card.dart
import 'package:flutter/material.dart';
import '../models/route_result.dart';

class RouteCard extends StatelessWidget {
  final String strategy;
  final RouteResult route;
  final VoidCallback? onTap;

  const RouteCard({
    Key? key,
    required this.strategy,
    required this.route,
    this.onTap,
  }) : super(key: key);

  @override
  Widget build(BuildContext context) {
    final info = _getStrategyInfo(strategy);

    return Card(
      elevation: 4,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
      ),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(16),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Header
              Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Row(
                    children: [
                      Text(
                        info['icon']!,
                        style: const TextStyle(fontSize: 24),
                      ),
                      const SizedBox(width: 8),
                      Text(
                        info['label']!,
                        style: const TextStyle(
                          fontSize: 18,
                          fontWeight: FontWeight.bold,
                        ),
                      ),
                    ],
                  ),
                  Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 12,
                      vertical: 6,
                    ),
                    decoration: BoxDecoration(
                      color: info['color'],
                      borderRadius: BorderRadius.circular(20),
                    ),
                    child: Text(
                      '${route.durationMinutes} min',
                      style: const TextStyle(
                        color: Colors.white,
                        fontWeight: FontWeight.bold,
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 16),

              // Details
              _buildDetailRow(
                icon: Icons.access_time,
                label: 'Durée',
                value: '${route.durationMinutes} minutes',
              ),
              const SizedBox(height: 8),
              _buildDetailRow(
                icon: Icons.directions_walk,
                label: 'Marche',
                value: '${route.walkDistanceMeters}m',
              ),
              const SizedBox(height: 8),
              _buildDetailRow(
                icon: Icons.swap_horiz,
                label: 'Transferts',
                value: '${route.transfers}',
              ),

              const SizedBox(height: 16),

              // Button
              SizedBox(
                width: double.infinity,
                child: ElevatedButton(
                  onPressed: onTap,
                  style: ElevatedButton.styleFrom(
                    backgroundColor: info['color'],
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(8),
                    ),
                  ),
                  child: const Text('Voir les détails'),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildDetailRow({
    required IconData icon,
    required String label,
    required String value,
  }) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Row(
          children: [
            Icon(icon, size: 16, color: Colors.grey[600]),
            const SizedBox(width: 8),
            Text(
              label,
              style: TextStyle(
                color: Colors.grey[600],
                fontSize: 14,
              ),
            ),
          ],
        ),
        Text(
          value,
          style: const TextStyle(
            fontWeight: FontWeight.w600,
            fontSize: 14,
          ),
        ),
      ],
    );
  }

  Map<String, dynamic> _getStrategyInfo(String strategy) {
    switch (strategy) {
      case 'simple':
        return {
          'label': 'Recommandé',
          'icon': '✓',
          'color': Colors.blue,
        };
      case 'fast':
        return {
          'label': 'Plus Rapide',
          'icon': '⚡',
          'color': Colors.orange,
        };
      case 'no_transfer':
        return {
          'label': 'Sans Transfert',
          'icon': '🛋️',
          'color': Colors.green,
        };
      case 'direct':
        return {
          'label': 'Direct',
          'icon': '➡️',
          'color': Colors.grey,
        };
      default:
        return {
          'label': strategy,
          'icon': '📍',
          'color': Colors.grey,
        };
    }
  }
}
```

## Widget de Détails d'Étape

```dart
// widgets/step_item.dart
import 'package:flutter/material.dart';
import '../models/step.dart';

class StepItem extends StatelessWidget {
  final Step step;
  final bool isLast;

  const StepItem({
    Key? key,
    required this.step,
    this.isLast = false,
  }) : super(key: key);

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Icon and line
        Column(
          children: [
            Container(
              width: 40,
              height: 40,
              decoration: BoxDecoration(
                color: _getStepColor().withOpacity(0.2),
                shape: BoxShape.circle,
              ),
              child: Center(
                child: Text(
                  _getStepIcon(),
                  style: const TextStyle(fontSize: 20),
                ),
              ),
            ),
            if (!isLast)
              Container(
                width: 2,
                height: 40,
                color: Colors.grey[300],
              ),
          ],
        ),
        const SizedBox(width: 16),

        // Content
        Expanded(
          child: Padding(
            padding: const EdgeInsets.only(bottom: 16),
            child: _buildStepContent(),
          ),
        ),
      ],
    );
  }

  Widget _buildStepContent() {
    switch (step.type) {
      case StepType.walk:
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Marcher ${step.distanceMeters}m',
              style: const TextStyle(
                fontWeight: FontWeight.bold,
                fontSize: 16,
              ),
            ),
            const SizedBox(height: 4),
            Text(
              'De ${step.fromStopName} à ${step.toStopName}',
              style: TextStyle(color: Colors.grey[600]),
            ),
            const SizedBox(height: 4),
            Text(
              '${step.durationMinutes} min',
              style: TextStyle(
                color: Colors.grey[500],
                fontSize: 12,
              ),
            ),
          ],
        );

      case StepType.ride:
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'Prendre ${step.routeName}',
              style: const TextStyle(
                fontWeight: FontWeight.bold,
                fontSize: 16,
              ),
            ),
            const SizedBox(height: 4),
            Text(
              'De ${step.fromStopName} à ${step.toStopName}',
              style: TextStyle(color: Colors.grey[600]),
            ),
            const SizedBox(height: 4),
            Text(
              '${step.numStops} arrêts • ${step.durationMinutes} min • ${step.mode}',
              style: TextStyle(
                color: Colors.grey[500],
                fontSize: 12,
              ),
            ),
          ],
        );

      case StepType.transfer:
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'Transfert',
              style: TextStyle(
                fontWeight: FontWeight.bold,
                fontSize: 16,
              ),
            ),
            const SizedBox(height: 4),
            Text(
              'À ${step.fromStopName}',
              style: TextStyle(color: Colors.grey[600]),
            ),
            const SizedBox(height: 4),
            Text(
              '${step.durationMinutes} min d\'attente',
              style: TextStyle(
                color: Colors.grey[500],
                fontSize: 12,
              ),
            ),
          ],
        );
    }
  }

  String _getStepIcon() {
    switch (step.type) {
      case StepType.walk:
        return '🚶';
      case StepType.ride:
        return '🚌';
      case StepType.transfer:
        return '🔄';
    }
  }

  Color _getStepColor() {
    switch (step.type) {
      case StepType.walk:
        return Colors.blue;
      case StepType.ride:
        return Colors.green;
      case StepType.transfer:
        return Colors.orange;
    }
  }
}
```

## Page Principale

```dart
// screens/route_search_screen.dart
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../providers/route_provider.dart';
import '../widgets/route_card.dart';
import '../models/coordinates.dart';

class RouteSearchScreen extends StatelessWidget {
  const RouteSearchScreen({Key? key}) : super(key: key);

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('🚌 PassBi - Planificateur'),
        elevation: 0,
      ),
      body: Consumer<RouteProvider>(
        builder: (context, provider, child) {
          if (provider.loading) {
            return const Center(
              child: CircularProgressIndicator(),
            );
          }

          if (provider.error != null) {
            return Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  const Icon(
                    Icons.error_outline,
                    size: 64,
                    color: Colors.red,
                  ),
                  const SizedBox(height: 16),
                  Text(
                    provider.error!,
                    style: const TextStyle(color: Colors.red),
                    textAlign: TextAlign.center,
                  ),
                  const SizedBox(height: 16),
                  ElevatedButton(
                    onPressed: () => provider.searchRoutes(),
                    child: const Text('Réessayer'),
                  ),
                ],
              ),
            );
          }

          if (provider.routes == null || provider.routes!.isEmpty) {
            return Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  const Icon(
                    Icons.search,
                    size: 64,
                    color: Colors.grey,
                  ),
                  const SizedBox(height: 16),
                  const Text('Aucun itinéraire'),
                  const SizedBox(height: 16),
                  ElevatedButton(
                    onPressed: () {
                      provider.setFrom(Coordinates(lat: 14.7167, lon: -17.4677));
                      provider.setTo(Coordinates(lat: 14.6928, lon: -17.4467));
                      provider.searchRoutes();
                    },
                    child: const Text('Rechercher'),
                  ),
                ],
              ),
            );
          }

          return ListView(
            padding: const EdgeInsets.all(16),
            children: [
              const Text(
                'Itinéraires Disponibles',
                style: TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.bold,
                ),
              ),
              const SizedBox(height: 16),
              ...provider.routes!.entries.map((entry) {
                return Padding(
                  padding: const EdgeInsets.only(bottom: 16),
                  child: RouteCard(
                    strategy: entry.key,
                    route: entry.value,
                    onTap: () {
                      // Navigate to details
                    },
                  ),
                );
              }).toList(),
            ],
          );
        },
      ),
    );
  }
}
```

## Application Main

```dart
// main.dart
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'services/passbi_service.dart';
import 'providers/route_provider.dart';
import 'screens/route_search_screen.dart';

void main() {
  runApp(const MyApp());
}

class MyApp extends StatelessWidget {
  const MyApp({Key? key}) : super(key: key);

  @override
  Widget build(BuildContext context) {
    final service = PassBiService(
      baseUrl: 'http://localhost:8080',
    );

    return MultiProvider(
      providers: [
        ChangeNotifierProvider(
          create: (_) => RouteProvider(service),
        ),
      ],
      child: MaterialApp(
        title: 'PassBi',
        theme: ThemeData(
          primarySwatch: Colors.blue,
          useMaterial3: true,
        ),
        home: const RouteSearchScreen(),
      ),
    );
  }
}
```

## Voir Aussi

- [React Examples](react.md) - Exemples React
- [JavaScript Examples](javascript.md) - Exemples JavaScript
- [Integration Guide](../../guides/integration-guide.md) - Guide complet
