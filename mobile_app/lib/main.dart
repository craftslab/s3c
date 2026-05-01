import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math';

import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

const String _defaultApiBaseUrl = String.fromEnvironment(
  'KIPUP_API_BASE_URL',
  defaultValue: 'http://localhost:8080/api/v1',
);

void main() {
  runApp(const KipupMobileApp());
}

class KipupMobileApp extends StatelessWidget {
  const KipupMobileApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Kipup Mobile',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF201912)),
        useMaterial3: true,
      ),
      home: const ActivationGate(),
    );
  }
}

class ActivationGate extends StatefulWidget {
  const ActivationGate({super.key});

  @override
  State<ActivationGate> createState() => _ActivationGateState();
}

class _ActivationGateState extends State<ActivationGate> {
  final KipupMobileController _controller = KipupMobileController();

  @override
  void initState() {
    super.initState();
    unawaited(_controller.initialize());
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _controller,
      builder: (context, _) {
        switch (_controller.state) {
          case AppState.loading:
            return const Scaffold(body: Center(child: CircularProgressIndicator()));
          case AppState.needsActivation:
            return ActivationScreen(controller: _controller);
          case AppState.blocked:
            return BlockedScreen(controller: _controller);
          case AppState.ready:
            return WorkspaceScreen(controller: _controller);
        }
      },
    );
  }
}

enum AppState { loading, needsActivation, blocked, ready }

class KipupMobileController extends ChangeNotifier {
  static const _prefsApiBaseUrl = 'kipup.mobile.apiBaseUrl';
  static const _prefsActivationToken = 'kipup.mobile.activationToken';
  static const _prefsActivationCode = 'kipup.mobile.activationCode';
  static const _prefsDeviceId = 'kipup.mobile.deviceId';
  static const _prefsReleaseTitle = 'kipup.mobile.releaseTitle';
  static const _prefsCollaborationTitle = 'kipup.mobile.collaborationTitle';
  static const _prefsExpiry = 'kipup.mobile.expiry';
  static const _prefsOfflineGraceSeconds = 'kipup.mobile.offlineGraceSeconds';
  static const _prefsLastValidatedAt = 'kipup.mobile.lastValidatedAt';

  AppState state = AppState.loading;
  String apiBaseUrl = _defaultApiBaseUrl;
  String activationCode = '';
  String activationToken = '';
  String deviceId = '';
  String releaseTitle = 'Kipup Mobile';
  String collaborationTitle = '';
  DateTime? expiresAt;
  DateTime? lastValidatedAt;
  int offlineGraceSeconds = 24 * 60 * 60;
  String blockingReason = '';
  bool offlineMode = false;

  Future<void> initialize() async {
    state = AppState.loading;
    notifyListeners();
    final prefs = await SharedPreferences.getInstance();
    apiBaseUrl = prefs.getString(_prefsApiBaseUrl) ?? _defaultApiBaseUrl;
    activationCode = prefs.getString(_prefsActivationCode) ?? '';
    activationToken = prefs.getString(_prefsActivationToken) ?? '';
    deviceId = prefs.getString(_prefsDeviceId) ?? _generateDeviceId();
    await prefs.setString(_prefsDeviceId, deviceId);
    releaseTitle = prefs.getString(_prefsReleaseTitle) ?? 'Kipup Mobile';
    collaborationTitle = prefs.getString(_prefsCollaborationTitle) ?? '';
    expiresAt = _readDate(prefs.getString(_prefsExpiry));
    lastValidatedAt = _readDate(prefs.getString(_prefsLastValidatedAt));
    offlineGraceSeconds = prefs.getInt(_prefsOfflineGraceSeconds) ?? offlineGraceSeconds;
    if (activationToken.isEmpty) {
      state = AppState.needsActivation;
      notifyListeners();
      return;
    }
    await validateStartup();
  }

  Future<void> activate({required String apiUrl, required String code}) async {
    final prefs = await SharedPreferences.getInstance();
    apiBaseUrl = apiUrl.trim().replaceAll(RegExp(r'/+$'), '');
    activationCode = code.trim();
    blockingReason = '';
    offlineMode = false;
    notifyListeners();

    final response = await _postJson(
      '$apiBaseUrl/mobile/download-links/$activationCode/activate',
      {
        'platform': Platform.isIOS ? 'ios' : 'android',
        'deviceId': deviceId,
        'deviceName': '${Platform.operatingSystem} device',
        'appVersion': '0.1.0'
      },
    );
    if (response.statusCode != HttpStatus.created) {
      throw Exception(_extractError(await response.transform(utf8.decoder).join()));
    }
    final payload = jsonDecode(await response.transform(utf8.decoder).join()) as Map<String, dynamic>;
    activationToken = payload['activationToken']?.toString() ?? '';
    releaseTitle = _mapString(payload, ['release', 'title']) ?? releaseTitle;
    collaborationTitle = payload['installation']?['collaborationTitle']?.toString() ?? '';
    expiresAt = _readDate(payload['expiresAt']?.toString());
    offlineGraceSeconds = (payload['offlineGraceSeconds'] as num?)?.toInt() ?? offlineGraceSeconds;
    lastValidatedAt = DateTime.now().toUtc();
    await prefs.setString(_prefsApiBaseUrl, apiBaseUrl);
    await prefs.setString(_prefsActivationCode, activationCode);
    await prefs.setString(_prefsActivationToken, activationToken);
    await prefs.setString(_prefsReleaseTitle, releaseTitle);
    await prefs.setString(_prefsCollaborationTitle, collaborationTitle);
    if (expiresAt != null) {
      await prefs.setString(_prefsExpiry, expiresAt!.toIso8601String());
    }
    await prefs.setInt(_prefsOfflineGraceSeconds, offlineGraceSeconds);
    await prefs.setString(_prefsLastValidatedAt, lastValidatedAt!.toIso8601String());
    await validateStartup();
  }

  Future<void> validateStartup() async {
    blockingReason = '';
    offlineMode = false;
    notifyListeners();
    try {
      final response = await _postJson(
        '$apiBaseUrl/mobile/installations/validate',
        {'activationToken': activationToken, 'deviceId': deviceId},
      );
      if (response.statusCode != HttpStatus.ok) {
        throw Exception(_extractError(await response.transform(utf8.decoder).join()));
      }
      final payload = jsonDecode(await response.transform(utf8.decoder).join()) as Map<String, dynamic>;
      final valid = payload['valid'] == true;
      expiresAt = _readDate(payload['expiresAt']?.toString());
      collaborationTitle = payload['collaborationTitle']?.toString() ?? collaborationTitle;
      releaseTitle = _mapString(payload, ['release', 'title']) ?? releaseTitle;
      offlineGraceSeconds = (payload['offlineGraceSeconds'] as num?)?.toInt() ?? offlineGraceSeconds;
      lastValidatedAt = _readDate(payload['serverTime']?.toString()) ?? DateTime.now().toUtc();
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString(_prefsReleaseTitle, releaseTitle);
      await prefs.setString(_prefsCollaborationTitle, collaborationTitle);
      await prefs.setInt(_prefsOfflineGraceSeconds, offlineGraceSeconds);
      await prefs.setString(_prefsLastValidatedAt, lastValidatedAt!.toIso8601String());
      if (expiresAt != null) {
        await prefs.setString(_prefsExpiry, expiresAt!.toIso8601String());
      }
      if (!valid) {
        await block(payload['reason']?.toString() ?? 'Access expired');
        return;
      }
      state = AppState.ready;
      notifyListeners();
    } catch (_) {
      if (_isWithinOfflineGrace()) {
        offlineMode = true;
        state = AppState.ready;
        notifyListeners();
        return;
      }
      await block('Validation failed and offline grace has elapsed.');
    }
  }

  Future<void> block(String reason) async {
    blockingReason = reason;
    state = AppState.blocked;
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_prefsActivationToken);
    await prefs.remove(_prefsReleaseTitle);
    await prefs.remove(_prefsCollaborationTitle);
    await prefs.remove(_prefsExpiry);
    await prefs.remove(_prefsLastValidatedAt);
    activationToken = '';
    releaseTitle = 'Kipup Mobile';
    collaborationTitle = '';
    expiresAt = null;
    lastValidatedAt = null;
    notifyListeners();
  }

  Future<void> reset() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_prefsActivationToken);
    await prefs.remove(_prefsActivationCode);
    await prefs.remove(_prefsReleaseTitle);
    await prefs.remove(_prefsCollaborationTitle);
    await prefs.remove(_prefsExpiry);
    await prefs.remove(_prefsLastValidatedAt);
    activationToken = '';
    activationCode = '';
    collaborationTitle = '';
    expiresAt = null;
    lastValidatedAt = null;
    blockingReason = '';
    offlineMode = false;
    state = AppState.needsActivation;
    notifyListeners();
  }

  bool _isWithinOfflineGrace() {
    if (lastValidatedAt == null) return false;
    return DateTime.now().toUtc().difference(lastValidatedAt!) <= Duration(seconds: offlineGraceSeconds);
  }

  Future<HttpClientResponse> _postJson(String url, Map<String, dynamic> payload) async {
    final client = HttpClient()..connectionTimeout = const Duration(seconds: 12);
    final request = await client.postUrl(Uri.parse(url));
    request.headers.contentType = ContentType.json;
    request.add(utf8.encode(jsonEncode(payload)));
    return request.close();
  }

  DateTime? _readDate(String? raw) {
    if (raw == null || raw.isEmpty) return null;
    return DateTime.tryParse(raw)?.toUtc();
  }

  String _generateDeviceId() {
    const alphabet = 'abcdefghijklmnopqrstuvwxyz0123456789';
    final random = Random.secure();
    return List.generate(24, (_) => alphabet[random.nextInt(alphabet.length)]).join();
  }

  String? _mapString(Map<String, dynamic> payload, List<String> path) {
    dynamic current = payload;
    for (final segment in path) {
      if (current is! Map<String, dynamic>) return null;
      current = current[segment];
    }
    return current?.toString();
  }

  String _extractError(String body) {
    if (body.isEmpty) return 'Request failed';
    try {
      final payload = jsonDecode(body) as Map<String, dynamic>;
      return payload['error']?.toString() ?? body;
    } catch (_) {
      return body;
    }
  }
}

class ActivationScreen extends StatefulWidget {
  const ActivationScreen({super.key, required this.controller});

  final KipupMobileController controller;

  @override
  State<ActivationScreen> createState() => _ActivationScreenState();
}

class _ActivationScreenState extends State<ActivationScreen> {
  late final TextEditingController _apiController;
  late final TextEditingController _codeController;
  bool _submitting = false;

  @override
  void initState() {
    super.initState();
    _apiController = TextEditingController(text: widget.controller.apiBaseUrl);
    _codeController = TextEditingController(text: widget.controller.activationCode);
  }

  @override
  void dispose() {
    _apiController.dispose();
    _codeController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Activate Kipup Mobile')),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text('Connect this mobile app to your expiring Kipup distribution link.'),
              const SizedBox(height: 20),
              TextField(
                controller: _apiController,
                decoration: const InputDecoration(labelText: 'API base URL', hintText: 'http://host:8080/api/v1'),
              ),
              const SizedBox(height: 16),
              TextField(
                controller: _codeController,
                decoration: const InputDecoration(labelText: 'Activation code'),
              ),
              const SizedBox(height: 20),
              FilledButton(
                onPressed: _submitting ? null : () async {
                  setState(() => _submitting = true);
                  try {
                    await widget.controller.activate(apiUrl: _apiController.text, code: _codeController.text);
                  } catch (error) {
                    if (!mounted) return;
                    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(error.toString())));
                  } finally {
                    if (mounted) setState(() => _submitting = false);
                  }
                },
                child: Text(_submitting ? 'Activating…' : 'Activate app'),
              ),
              const SizedBox(height: 20),
              Text('Device ID: ${widget.controller.deviceId}', style: Theme.of(context).textTheme.bodySmall),
            ],
          ),
        ),
      ),
    );
  }
}

class WorkspaceScreen extends StatelessWidget {
  const WorkspaceScreen({super.key, required this.controller});

  final KipupMobileController controller;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(controller.releaseTitle),
        actions: [
          IconButton(
            onPressed: () => controller.validateStartup(),
            icon: const Icon(Icons.refresh),
          ),
          IconButton(
            onPressed: () => controller.reset(),
            icon: const Icon(Icons.logout),
          ),
        ],
      ),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(20),
          children: [
            if (controller.offlineMode)
              const Card(
                child: ListTile(
                  leading: Icon(Icons.wifi_off),
                  title: Text('Offline grace mode'),
                  subtitle: Text('The app is temporarily unlocked using the last successful validation.'),
                ),
              ),
            Card(
              child: ListTile(
                leading: const Icon(Icons.workspace_premium_outlined),
                title: Text(controller.releaseTitle),
                subtitle: Text(controller.collaborationTitle.isEmpty ? 'No linked collaboration room' : controller.collaborationTitle),
              ),
            ),
            Card(
              child: ListTile(
                leading: const Icon(Icons.timer_outlined),
                title: const Text('Access expires'),
                subtitle: Text(controller.expiresAt?.toLocal().toString() ?? 'Unknown'),
              ),
            ),
            Card(
              child: ListTile(
                leading: const Icon(Icons.verified_user_outlined),
                title: const Text('Startup validation'),
                subtitle: Text('Every launch calls /mobile/installations/validate and blocks the app after expiry or revocation.'),
              ),
            ),
            Card(
              child: ListTile(
                leading: const Icon(Icons.info_outline),
                title: const Text('MVP client scaffold'),
                subtitle: const Text('Hook collaboration, files, chat, audio, and video modules into this validated shell to reach full feature parity.'),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class BlockedScreen extends StatelessWidget {
  const BlockedScreen({super.key, required this.controller});

  final KipupMobileController controller;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(Icons.block, size: 56),
                const SizedBox(height: 16),
                const Text('App access expired', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w600)),
                const SizedBox(height: 12),
                Text(controller.blockingReason, textAlign: TextAlign.center),
                const SizedBox(height: 20),
                FilledButton(
                  onPressed: controller.reset,
                  child: const Text('Reset activation'),
                )
              ],
            ),
          ),
        ),
      ),
    );
  }
}
