# Routing Strategies Deep Dive

Complete technical guide to PassBi's four routing strategies and the underlying algorithms.

## Table of Contents

1. [Overview](#overview)
2. [Routing Algorithm (A*)](#routing-algorithm-a)
3. [Strategy 1: No Transfer](#strategy-1-no-transfer)
4. [Strategy 2: Direct](#strategy-2-direct)
5. [Strategy 3: Simple](#strategy-3-simple)
6. [Strategy 4: Fast](#strategy-4-fast)
7. [Strategy Comparison](#strategy-comparison)
8. [Use Case Recommendations](#use-case-recommendations)
9. [Performance Characteristics](#performance-characteristics)
10. [Advanced Topics](#advanced-topics)

---

## Overview

PassBi uses an **A* pathfinding algorithm** with **custom cost functions** to compute optimal routes. The system runs **4 strategies in parallel**, each with different cost weighting and constraints.

### Key Concepts

**Graph Structure:**
- **Nodes**: (stop, route) pairs - each node represents a specific stop served by a specific route
- **Edges**: Connections between nodes (WALK, RIDE, TRANSFER)
- **Cost**: Each edge has time, walking distance, and transfer penalties

**Edge Types:**
- **WALK**: Walking between two stops or to/from origin/destination
- **RIDE**: Riding a transit vehicle along a route
- **TRANSFER**: Changing from one route to another at the same stop

**Strategy Variables:**
- **Edge cost function**: How edges are weighted during pathfinding
- **Transfer limit**: Maximum number of transfers allowed
- **Node exploration limit**: Maximum nodes to explore before giving up

---

## Routing Algorithm (A*)

PassBi uses **A\* (A-star)** pathfinding, a best-first search algorithm that finds the optimal path.

### A* Basics

**Formula:** `f(n) = g(n) + h(n)`

Where:
- `f(n)` = Total estimated cost of path through node n
- `g(n)` = Actual cost from start to node n
- `h(n)` = Heuristic estimated cost from node n to goal

**Heuristic (h):**
PassBi uses straight-line distance (haversine) from current node to destination, converted to estimated time:

```
h(n) = haversineDistance(node, goal) / walkingSpeed
```

This heuristic is:
- **Admissible**: Never overestimates (ensures optimal solution)
- **Consistent**: Satisfies triangle inequality

### Algorithm Flow

```
1. Find nearest nodes to origin (within MAX_WALK_DISTANCE)
2. Add them to open set (priority queue)
3. While open set not empty:
   a. Pop node with lowest f-score
   b. If node is at destination stop → reconstruct path
   c. For each outgoing edge from node:
      - Calculate new g-score using strategy's cost function
      - If better than previous → update and add to open set
4. Return best path or null if no path found
```

### Example Path Construction

**Journey:** Origin → Stop A → (Ride Route 1) → Stop B → (Transfer) → Stop B → (Ride Route 2) → Stop C → Destination

**Graph nodes:**
1. `(origin, virtual)` - Starting point
2. `(Stop A, Route 1)` - Board Route 1 at Stop A
3. `(Stop B, Route 1)` - Still on Route 1 at Stop B
4. `(Stop B, Route 2)` - After transferring to Route 2 at Stop B
5. `(Stop C, Route 2)` - On Route 2 at Stop C
6. `(destination, virtual)` - End point

**Edges:**
1. WALK from origin to `(Stop A, Route 1)`
2. RIDE along Route 1 from `(Stop A, Route 1)` to `(Stop B, Route 1)`
3. TRANSFER from `(Stop B, Route 1)` to `(Stop B, Route 2)`
4. RIDE along Route 2 from `(Stop B, Route 2)` to `(Stop C, Route 2)`
5. WALK from `(Stop C, Route 2)` to destination

---

## Strategy 1: No Transfer

**Goal:** Absolutely zero transfers - single transit line only

### Cost Function

```go
func EdgeCost(edge) int {
    switch edge.Type {
    case TRANSFER:
        return 999999999  // Effectively infinite - no transfers allowed
    case WALK:
        return edge.Time * 5  // Moderate walk penalty
    case RIDE:
        return edge.Time
    }
}
```

**Formula:**
- Transfer cost: **999,999,999** (effectively infinite)
- Walk cost: **Time × 5**
- Ride cost: **Time only**

### Constraints

- **Max transfers:** 0 (enforced by infinite cost)
- **Max explored nodes:** 3,000
- **Stops immediately if:** Any transfer is encountered

### Behavior

This strategy will:
- Only find routes using a **single transit line**
- Allow moderate walking to reach the right line
- Fail (return no route) if no single line connects origin and destination
- Be less aggressive in exploration (lower node limit)

### When to Use

**Best for:**
- Users with heavy luggage or shopping
- Elderly or mobility-impaired passengers
- Parents with young children or strollers
- Long-distance trips where comfort > speed
- Users unfamiliar with the transit system

**Not recommended for:**
- Long distances requiring multiple lines
- Areas with limited single-line connectivity
- Time-sensitive journeys

### Example Scenarios

**Good match:**
```
Origin: Near Stop A (served by Line 1)
Destination: Near Stop Z (also served by Line 1)
→ Walk to Stop A → Ride Line 1 (15 stops) → Walk to destination
Result: ✓ Single line, no transfers
```

**Poor match:**
```
Origin: East side (served by Line 1)
Destination: West side (only served by Line 3, no overlap)
→ Would require transfer from Line 1 to Line 3
Result: ✗ No route found (transfer required)
```

### Cost Analysis

**Example journey:**
- Walk 200m (140s): Cost = 140 × 5 = **700**
- Ride 15 minutes (900s): Cost = **900**
- Total: **1,600**

If a transfer were needed:
- Transfer penalty: **999,999,999**
- Total: **999,999,999+** → Path rejected

---

## Strategy 2: Direct

**Goal:** Minimize or eliminate transfers (simplicity focused)

### Cost Function

```go
func EdgeCost(edge) int {
    switch edge.Type {
    case TRANSFER:
        return 999999  // Very high - avoid transfers
    case WALK:
        return edge.Time * 10  // Heavy walk penalty
    case RIDE:
        return edge.Time
    }
}
```

**Formula:**
- Transfer cost: **999,999** (very high but not infinite)
- Walk cost: **Time × 10**
- Ride cost: **Time only**

### Constraints

- **Max transfers:** 0 (practically, due to high cost)
- **Max explored nodes:** 5,000
- **Stops if:** Any transfer is made OR too many nodes explored

### Behavior

This strategy will:
- **Strongly prefer** zero-transfer routes
- **Heavily penalize** walking (more than no_transfer)
- Only accept transfers in extreme cases (very long alternative)
- Prioritize simplicity over speed

### Differences from No Transfer

| Aspect | No Transfer | Direct |
|--------|-------------|--------|
| Transfer penalty | ∞ (impossible) | 999,999 (very high) |
| Walk penalty | Time × 5 | Time × 10 (stricter) |
| Max nodes | 3,000 | 5,000 |
| Will ever allow transfer? | Never | Theoretically yes, in extreme cases |

### When to Use

**Best for:**
- Users who strongly prefer simplicity
- First-time transit users
- Quick trips where direct routes are available
- Users who want to minimize walking

**Not recommended for:**
- Trips where direct routes are much longer
- Areas with sparse direct connectivity

### Example Comparison

**Scenario:** Origin to Destination

**Option A (Direct, no transfer):**
- Walk 50m (40s): Cost = 40 × 10 = **400**
- Ride 20 minutes (1200s): Cost = **1,200**
- Total: **1,600**

**Option B (With transfer, faster):**
- Walk 200m (140s): Cost = 140 × 10 = **1,400**
- Ride 8 minutes (480s): Cost = **480**
- Transfer (180s): Cost = **999,999**
- Ride 5 minutes (300s): Cost = **300**
- Total: **1,000,179** ← Much higher cost

**Result:** Direct strategy chooses Option A (longer but simpler)

---

## Strategy 3: Simple

**Goal:** Balance time, walking distance, and transfers

**✅ RECOMMENDED AS DEFAULT**

### Cost Function

```go
func EdgeCost(edge) int {
    cost := edge.Time

    if edge.Type == WALK {
        cost += edge.WalkDistance * 2  // Walking is 2x as costly as riding
    }

    if edge.Type == TRANSFER {
        cost += 180 * edge.TransferCount  // 3 minutes penalty per transfer
    }

    return cost
}
```

**Formula:**
- Cost = **Time + (Walk distance × 2) + (Transfers × 180 seconds)**

### Constraints

- **Max transfers:** 2
- **Max explored nodes:** 10,000
- **Stops if:** More than 2 transfers OR too many nodes explored

### Behavior

This strategy will:
- **Balance** all factors (time, walking, transfers)
- Allow up to **2 transfers** if they save significant time
- Penalize walking moderately (2x time cost)
- Add **3-minute penalty** per transfer
- Find the "best overall" route for most users

### Cost Breakdown Example

**Journey with 1 transfer:**
- Walk 250m (175s): Cost = 175 + (250 × 2) = **675**
- Ride 7 minutes (420s): Cost = **420**
- Transfer (180s): Cost = 180 + (180 × 1) = **360**
- Ride 6 minutes (360s): Cost = **360**
- Walk 100m (70s): Cost = 70 + (100 × 2) = **270**
- **Total: 2,085** (≈ 35 minutes equivalent cost)

**Alternative direct route (no transfer):**
- Walk 100m (70s): Cost = 70 + (100 × 2) = **270**
- Ride 25 minutes (1500s): Cost = **1,500**
- Walk 50m (35s): Cost = 35 + (50 × 2) = **135**
- **Total: 1,905** (≈ 32 minutes equivalent cost)

**Result:** Simple strategy chooses the direct route (lower total cost)

### When to Use

**Best for:**
- **General-purpose routing** (recommended default)
- Most users and use cases
- When you want a good balance
- Unknown user preferences

**Always recommend this as the primary option** in your UI.

---

## Strategy 4: Fast

**Goal:** Minimize total travel time (speed focused)

### Cost Function

```go
func EdgeCost(edge) int {
    return edge.Time  // Only consider actual time
}
```

**Formula:**
- Cost = **Time only**
- Walking and transfers are not penalized (except for their actual time)

### Constraints

- **Max transfers:** 3
- **Max explored nodes:** 10,000
- **Stops if:** More than 3 transfers OR too many nodes explored

### Behavior

This strategy will:
- Find the **absolute fastest** route by clock time
- Ignore walking distance (beyond the time it takes)
- Ignore transfer inconvenience (beyond the wait time)
- May require significant walking or multiple transfers

### Cost Analysis Example

**Fast route with multiple transfers:**
- Walk 400m (280s): Cost = **280**
- Ride 4 minutes (240s): Cost = **240**
- Transfer (180s): Cost = **180**
- Ride 3 minutes (180s): Cost = **180**
- Transfer (180s): Cost = **180**
- Ride 4 minutes (240s): Cost = **240**
- Walk 100m (70s): Cost = **70**
- **Total: 1,370 seconds (≈ 23 minutes)**

**Simple route (fewer transfers, less walking):**
- Walk 150m (105s): Cost = 105 + (150 × 2) = **405**
- Ride 12 minutes (720s): Cost = **720**
- Transfer (180s): Cost = 180 + 180 = **360**
- Ride 8 minutes (480s): Cost = **480**
- **Total: 1,965** (simple strategy) vs **1,370** (fast strategy)

**Result:** Fast strategy chooses the route with more transfers because it's actually faster.

### When to Use

**Best for:**
- Commuters in a hurry
- Time-sensitive trips (appointments, meetings)
- Users who prioritize speed over comfort
- "Express mode" in your app

**Not recommended for:**
- Users with mobility constraints
- Users with luggage
- Elderly passengers
- Unfamiliar users who may get confused by multiple transfers

---

## Strategy Comparison

### Side-by-Side Comparison

| Feature | No Transfer | Direct | Simple | Fast |
|---------|-------------|--------|--------|------|
| **Primary Goal** | Zero transfers | Minimal transfers | Balanced | Fastest time |
| **Transfer Cost** | ∞ | 999,999 | +180s each | Time only |
| **Walk Penalty** | Time × 5 | Time × 10 | Distance × 2 | None |
| **Ride Cost** | Time | Time | Time | Time |
| **Max Transfers** | 0 | 0 (practically) | 2 | 3 |
| **Max Nodes** | 3,000 | 5,000 | 10,000 | 10,000 |
| **Best For** | Comfort | Simplicity | General use | Speed |
| **Walking Tolerance** | Moderate | Low | Moderate | High |
| **Complexity** | Lowest | Low | Medium | Highest |

### Typical Results

For a 5km journey:

| Strategy | Duration | Walking | Transfers | Example |
|----------|----------|---------|-----------|---------|
| no_transfer | 35 min | 200m | 0 | Single line, longer route |
| direct | 28 min | 150m | 0 | Shortest direct line |
| simple | 22 min | 300m | 1 | One optimal transfer |
| fast | 18 min | 450m | 2 | Multiple quick hops |

---

## Use Case Recommendations

### By User Type

**Tourist / First-Time User:**
- Primary: `direct`
- Secondary: `simple`
- Avoid: `fast` (too complex)

**Daily Commuter:**
- Primary: `fast`
- Secondary: `simple`
- Use `no_transfer` for comfort days

**Elderly / Mobility-Impaired:**
- Primary: `no_transfer`
- Secondary: `direct`
- Avoid: `fast`

**Parent with Children:**
- Primary: `no_transfer`
- Secondary: `simple`

**Business Traveler:**
- Primary: `fast`
- Secondary: `simple`

**General Public (Unknown):**
- Primary: `simple` ← **Default recommendation**
- Show all 4 options and let user choose

### By Trip Type

**Short Distance (<2km):**
- `simple` or `fast` (similar results)

**Medium Distance (2-10km):**
- All strategies may give different results
- Show all 4 options

**Long Distance (>10km):**
- `fast` usually significantly better
- `no_transfer` may not find a route

### By Time of Day

**Rush Hour (Crowded):**
- `no_transfer` (avoid packed transfer stations)
- `direct` (simplicity in chaos)

**Off-Peak:**
- `fast` (can handle transfers comfortably)
- `simple` (balanced)

### UI Presentation Examples

**Badge System:**
```javascript
const strategyLabels = {
  simple: { badge: '✓', label: 'Recommended', color: 'blue' },
  fast: { badge: '⚡', label: 'Fastest', color: 'yellow' },
  no_transfer: { badge: '🛋️', label: 'Most Comfortable', color: 'green' },
  direct: { badge: '➡️', label: 'Direct', color: 'gray' }
};
```

**Sort Order:**
1. `simple` (recommended) - always first
2. `fast` - if significantly faster (>20% time savings)
3. `no_transfer` - if available and similar time
4. `direct` - as alternative option

---

## Performance Characteristics

### Computational Cost

**Node Exploration:**
- `no_transfer`: Fastest (max 3,000 nodes, early termination)
- `direct`: Fast (max 5,000 nodes)
- `simple`: Moderate (max 10,000 nodes)
- `fast`: Moderate (max 10,000 nodes)

**Average Response Times** (single strategy):
- P50: <100ms
- P95: <200ms
- P99: <400ms

**Parallel Execution** (all 4 strategies):
- P50: <150ms
- P95: <500ms
- P99: <800ms

### Cache Effectiveness

**Cache Hit Rates** (10-minute TTL):
- Typical: 60-70% for repeated searches
- Rush hour: 40-50% (more diverse queries)
- Off-peak: 70-80% (fewer unique searches)

### Memory Usage

Per route calculation:
- Graph data: Loaded on-demand (lazy loading)
- Path state: ~1-2KB per explored node
- Result: ~5-10KB per complete route

---

## Advanced Topics

### Custom Strategies

You can conceptually define custom strategies by understanding the cost function:

**Example: "Minimal Walking" Strategy**
```javascript
// Hypothetical cost function
cost = time + (walking_distance * 20) + (transfers * 60)
// Walking heavily penalized, transfers moderately penalized
```

**Example: "Minimal Transfers" Strategy**
```javascript
cost = time + (walking_distance * 1) + (transfers * 600)
// Transfers heavily penalized, walking lightly penalized
```

### Strategy Selection Algorithm

Automatically choose strategy based on context:

```javascript
function selectStrategy(user, trip) {
  // User with accessibility needs
  if (user.accessibilityNeeds) {
    return 'no_transfer';
  }

  // Rush hour preference
  if (isRushHour() && user.prefersComfort) {
    return 'direct';
  }

  // Time-sensitive trip
  if (trip.urgency === 'high') {
    return 'fast';
  }

  // Default
  return 'simple';
}
```

### Multi-Objective Optimization

Compare strategies using weighted scoring:

```javascript
function scoreRoute(route, weights = { time: 0.5, walking: 0.3, transfers: 0.2 }) {
  const timeScore = 1 / (route.duration_seconds / 60); // Inverse of minutes
  const walkScore = 1 / (1 + route.walk_distance_meters / 100); // Penalty for 100m
  const transferScore = 1 / (1 + route.transfers); // Penalty per transfer

  return (
    weights.time * timeScore +
    weights.walking * walkScore +
    weights.transfers * transferScore
  );
}

// Find best overall route
const scored = Object.entries(routes).map(([strategy, route]) => ({
  strategy,
  route,
  score: scoreRoute(route, userWeights)
}));

const best = scored.sort((a, b) => b.score - a.score)[0];
```

---

## See Also

- [Integration Guide](integration-guide.md) - Using strategies in your app
- [API Reference](../api/openapi.yaml) - Complete API specification
- [Data Models](../api/reference/data-models.md) - Route data structures
- [Code Examples](../api/examples/) - Implementation examples

---

**Understanding these strategies will help you provide the best routing experience for your users! 🚌⚡🎯**
