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
const int _initialPollDelaySeconds = 5;
const int _maxPollDelaySeconds = 30;

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
  static const _prefsCollaborationToken = 'kipup.mobile.collaborationToken';
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
  String collaborationToken = '';
  DateTime? expiresAt;
  DateTime? lastValidatedAt;
  int offlineGraceSeconds = 24 * 60 * 60;
  String blockingReason = '';
  bool offlineMode = false;
  bool chatLoading = false;
  String chatUsername = '';
  int unreadMessages = 0;
  String lastReadMessageId = '';
  List<String> mentionableUsers = const [];
  List<Map<String, dynamic>> collaborationMessages = const [];

  Timer? _pollTimer;
  int _pollFailures = 0;

  Future<void> initialize() async {
    state = AppState.loading;
    notifyListeners();
    final prefs = await SharedPreferences.getInstance();
    apiBaseUrl = prefs.getString(_prefsApiBaseUrl) ?? _defaultApiBaseUrl;
    activationCode = prefs.getString(_prefsActivationCode) ?? '';
    activationToken = prefs.getString(_prefsActivationToken) ?? '';
    deviceId = prefs.getString(_prefsDeviceId) ?? _generateDeviceId();
    collaborationToken = prefs.getString(_prefsCollaborationToken) ?? '';
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
    final payload = await _decodeJsonResponse(response, acceptedStatusCodes: {HttpStatus.created});
    activationToken = payload['activationToken']?.toString() ?? '';
    releaseTitle = _mapString(payload, ['release', 'title']) ?? releaseTitle;
    collaborationTitle = _mapString(payload, ['installation', 'collaborationTitle']) ?? '';
    collaborationToken =
        _mapString(payload, ['installation', 'collaborationToken']) ?? _mapString(payload, ['release', 'collaborationToken']) ?? '';
    expiresAt = _readDate(payload['expiresAt']?.toString());
    offlineGraceSeconds = (payload['offlineGraceSeconds'] as num?)?.toInt() ?? offlineGraceSeconds;
    lastValidatedAt = DateTime.now().toUtc();
    await prefs.setString(_prefsApiBaseUrl, apiBaseUrl);
    await prefs.setString(_prefsActivationCode, activationCode);
    await prefs.setString(_prefsActivationToken, activationToken);
    await prefs.setString(_prefsReleaseTitle, releaseTitle);
    await prefs.setString(_prefsCollaborationTitle, collaborationTitle);
    await prefs.setString(_prefsCollaborationToken, collaborationToken);
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
      final payload = await _decodeJsonResponse(response);
      final valid = payload['valid'] == true;
      expiresAt = _readDate(payload['expiresAt']?.toString());
      collaborationTitle = payload['collaborationTitle']?.toString() ?? collaborationTitle;
      collaborationToken = payload['collaborationToken']?.toString() ?? collaborationToken;
      releaseTitle = _mapString(payload, ['release', 'title']) ?? releaseTitle;
      offlineGraceSeconds = (payload['offlineGraceSeconds'] as num?)?.toInt() ?? offlineGraceSeconds;
      lastValidatedAt = _readDate(payload['serverTime']?.toString()) ?? DateTime.now().toUtc();
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString(_prefsReleaseTitle, releaseTitle);
      await prefs.setString(_prefsCollaborationTitle, collaborationTitle);
      await prefs.setString(_prefsCollaborationToken, collaborationToken);
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
      await loadCollaborationSession(silent: true);
      _startPolling();
      notifyListeners();
    } catch (_) {
      if (_isWithinOfflineGrace()) {
        offlineMode = true;
        state = AppState.ready;
        await loadCollaborationSession(silent: true);
        _startPolling();
        notifyListeners();
        return;
      }
      await block('Validation failed and offline grace has elapsed.');
    }
  }

  Future<void> loadCollaborationSession({bool silent = false}) async {
    if (collaborationToken.isEmpty || activationToken.isEmpty) {
      collaborationMessages = const [];
      unreadMessages = 0;
      mentionableUsers = const [];
      chatUsername = '';
      notifyListeners();
      return;
    }
    if (!silent) {
      chatLoading = true;
      notifyListeners();
    }
    try {
      final response = await _postJson(
        '$apiBaseUrl/mobile/collaboration/session',
        {'activationToken': activationToken, 'deviceId': deviceId},
      );
      final payload = await _decodeJsonResponse(response);
      collaborationTitle = payload['title']?.toString() ?? collaborationTitle;
      chatUsername = payload['currentUsername']?.toString() ?? chatUsername;
      unreadMessages = (payload['unreadCount'] as num?)?.toInt() ?? unreadMessages;
      lastReadMessageId = payload['lastReadMessageId']?.toString() ?? '';
      mentionableUsers = ((payload['mentionableUsers'] as List?) ?? const [])
          .map((item) => item.toString())
          .toList(growable: false);
      collaborationMessages = ((payload['messages'] as List?) ?? const [])
          .map((item) => Map<String, dynamic>.from(item as Map))
          .toList(growable: false);
    } finally {
      chatLoading = false;
      notifyListeners();
    }
  }

  Future<void> sendCollaborationMessage({
    required String content,
    String replyToId = '',
    String quickReply = '',
    String type = 'markdown',
  }) async {
    final response = await _postJson(
      '$apiBaseUrl/mobile/collaboration/messages',
      {
        'activationToken': activationToken,
        'deviceId': deviceId,
        'content': content,
        'replyToId': replyToId,
        'quickReply': quickReply,
        'type': type,
        'mentionedUsers': _extractMentions(content),
      },
    );
    await _decodeJsonResponse(response, acceptedStatusCodes: {HttpStatus.created});
    await loadCollaborationSession(silent: true);
  }

  Future<void> markCollaborationRead([String messageId = '']) async {
    final response = await _postJson(
      '$apiBaseUrl/mobile/collaboration/read',
      {
        'activationToken': activationToken,
        'deviceId': deviceId,
        'messageId': messageId.isEmpty ? _latestMessageId() : messageId,
      },
    );
    final payload = await _decodeJsonResponse(response);
    unreadMessages = (payload['unreadCount'] as num?)?.toInt() ?? 0;
    lastReadMessageId = payload['lastReadMessageId']?.toString() ?? _latestMessageId();
    notifyListeners();
  }

  Future<void> toggleReaction(String messageId, String emoji) async {
    final response = await _postJson(
      '$apiBaseUrl/mobile/collaboration/messages/$messageId/reactions',
      {
        'activationToken': activationToken,
        'deviceId': deviceId,
        'emoji': emoji,
      },
    );
    await _decodeJsonResponse(response);
    await loadCollaborationSession(silent: true);
  }

  Future<void> recallMessage(String messageId) async {
    final response = await _postJson(
      '$apiBaseUrl/mobile/collaboration/messages/$messageId/recall',
      {
        'activationToken': activationToken,
        'deviceId': deviceId,
      },
    );
    await _decodeJsonResponse(response);
    await loadCollaborationSession(silent: true);
  }

  Future<void> deleteMessage(String messageId) async {
    final response = await _requestJson(
      'DELETE',
      '$apiBaseUrl/mobile/collaboration/messages/$messageId',
      {
        'activationToken': activationToken,
        'deviceId': deviceId,
      },
    );
    await _decodeJsonResponse(response);
    await loadCollaborationSession(silent: true);
  }

  Future<String> exportCollaborationTranscript(String format) async {
    final url = Uri.parse(
      '$apiBaseUrl/mobile/collaboration/export?activationToken=$activationToken&deviceId=$deviceId&format=$format',
    );
    final client = HttpClient()..connectionTimeout = const Duration(seconds: 12);
    final request = await client.getUrl(url);
    final response = await request.close();
    if (response.statusCode != HttpStatus.ok) {
      throw Exception(_extractError(await response.transform(utf8.decoder).join()));
    }
    final bytes = await response.fold<List<int>>(<int>[], (buffer, chunk) {
      buffer.addAll(chunk);
      return buffer;
    });
    final disposition = response.headers.value(HttpHeaders.contentDispositionHeader) ?? '';
    final match = RegExp(r'filename="?([^";]+)"?').firstMatch(disposition);
    final filename = _sanitizeExportFileName(match?.group(1) ?? 'collaboration.$format');
    final file = File('${Directory.systemTemp.path}/$filename');
    await file.writeAsBytes(bytes, flush: true);
    return file.path;
  }

  Future<void> block(String reason) async {
    _pollTimer?.cancel();
    _pollFailures = 0;
    blockingReason = reason;
    state = AppState.blocked;
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_prefsActivationToken);
    await prefs.remove(_prefsReleaseTitle);
    await prefs.remove(_prefsCollaborationTitle);
    await prefs.remove(_prefsCollaborationToken);
    await prefs.remove(_prefsExpiry);
    await prefs.remove(_prefsLastValidatedAt);
    activationToken = '';
    releaseTitle = 'Kipup Mobile';
    collaborationTitle = '';
    collaborationToken = '';
    expiresAt = null;
    lastValidatedAt = null;
    collaborationMessages = const [];
    mentionableUsers = const [];
    unreadMessages = 0;
    notifyListeners();
  }

  Future<void> reset() async {
    _pollTimer?.cancel();
    _pollFailures = 0;
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_prefsActivationToken);
    await prefs.remove(_prefsActivationCode);
    await prefs.remove(_prefsReleaseTitle);
    await prefs.remove(_prefsCollaborationTitle);
    await prefs.remove(_prefsCollaborationToken);
    await prefs.remove(_prefsExpiry);
    await prefs.remove(_prefsLastValidatedAt);
    activationToken = '';
    activationCode = '';
    collaborationTitle = '';
    collaborationToken = '';
    expiresAt = null;
    lastValidatedAt = null;
    blockingReason = '';
    offlineMode = false;
    collaborationMessages = const [];
    mentionableUsers = const [];
    unreadMessages = 0;
    state = AppState.needsActivation;
    notifyListeners();
  }

  bool _isWithinOfflineGrace() {
    if (lastValidatedAt == null) return false;
    return DateTime.now().toUtc().difference(lastValidatedAt!) <= Duration(seconds: offlineGraceSeconds);
  }

  void _startPolling() {
    _pollTimer?.cancel();
    if (collaborationToken.isEmpty) return;
    _scheduleNextPoll();
  }

  void _scheduleNextPoll() {
    _pollTimer?.cancel();
    if (collaborationToken.isEmpty) return;
    final delaySeconds = _pollFailures <= 0 ? _initialPollDelaySeconds : min(_maxPollDelaySeconds, _initialPollDelaySeconds * (_pollFailures + 1));
    _pollTimer = Timer(Duration(seconds: delaySeconds), () {
      unawaited(_pollCollaborationSession());
    });
  }

  Future<void> _pollCollaborationSession() async {
    try {
      await loadCollaborationSession(silent: true);
      _pollFailures = 0;
    } catch (_) {
      _pollFailures++;
    } finally {
      _scheduleNextPoll();
    }
  }

  List<String> _extractMentions(String content) {
    final matches = RegExp(r'(^|\s)@([A-Za-z0-9._-]{3,64})').allMatches(content);
    return matches.map((match) => match.group(2) ?? '').where((item) => item.isNotEmpty).toSet().toList();
  }

  String _latestMessageId() => collaborationMessages.isEmpty ? '' : collaborationMessages.last['id']?.toString() ?? '';

  Future<HttpClientResponse> _postJson(String url, Map<String, dynamic> payload) async {
    return _requestJson('POST', url, payload);
  }

  Future<HttpClientResponse> _requestJson(String method, String url, Map<String, dynamic> payload) async {
    final client = HttpClient()..connectionTimeout = const Duration(seconds: 12);
    final request = await client.openUrl(method, Uri.parse(url));
    request.headers.contentType = ContentType.json;
    request.add(utf8.encode(jsonEncode(payload)));
    return request.close();
  }

  Future<Map<String, dynamic>> _decodeJsonResponse(
    HttpClientResponse response, {
    Set<int> acceptedStatusCodes = const {HttpStatus.ok},
  }) async {
    final body = await response.transform(utf8.decoder).join();
    if (!acceptedStatusCodes.contains(response.statusCode)) {
      throw Exception(_extractError(body));
    }
    if (body.isEmpty) return <String, dynamic>{};
    return jsonDecode(body) as Map<String, dynamic>;
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

  String _sanitizeExportFileName(String value) {
    final sanitized = value.replaceAll(RegExp(r'[^A-Za-z0-9._-]+'), '-');
    return sanitized.isEmpty ? 'collaboration-export' : sanitized;
  }

  @override
  void dispose() {
    _pollTimer?.cancel();
    super.dispose();
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
                onPressed: _submitting
                    ? null
                    : () async {
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

class WorkspaceScreen extends StatefulWidget {
  const WorkspaceScreen({super.key, required this.controller});

  final KipupMobileController controller;

  @override
  State<WorkspaceScreen> createState() => _WorkspaceScreenState();
}

class _WorkspaceScreenState extends State<WorkspaceScreen> {
  final TextEditingController _messageController = TextEditingController();
  Map<String, dynamic>? _replyTarget;
  String? _mentionQuery;

  static const _quickReplies = [
    {'label': 'Acknowledge', 'content': 'Acknowledged.', 'quickReply': '✅ Acknowledged'},
    {'label': 'On my way', 'content': 'On my way.', 'quickReply': '🚀 On my way'},
    {'label': 'Need info', 'content': 'I need more information.', 'quickReply': '❓ Need info'},
  ];
  static const _reactions = ['👍', '🎯', '🔥', '✅'];
  static final RegExp _mentionDraftPattern = RegExp(r'(^|\s)@([A-Za-z0-9._-]{0,64})$');

  @override
  void initState() {
    super.initState();
    _messageController.addListener(_syncMentionSuggestions);
  }

  @override
  void dispose() {
    _messageController.removeListener(_syncMentionSuggestions);
    _messageController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final controller = widget.controller;
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
            if (controller.collaborationToken.isNotEmpty) ...[
              Card(
                child: Padding(
                  padding: const EdgeInsets.all(16),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          const Expanded(
                            child: Text('Mobile collaboration', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600)),
                          ),
                          if (controller.unreadMessages > 0)
                            Chip(label: Text('${controller.unreadMessages} unread')),
                          PopupMenuButton<String>(
                            onSelected: (value) async {
                              try {
                                final path = await controller.exportCollaborationTranscript(value);
                                if (!mounted) return;
                                ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('Saved export to $path')));
                              } catch (error) {
                                if (!mounted) return;
                                ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(error.toString())));
                              }
                            },
                            itemBuilder: (context) => const [
                              PopupMenuItem(value: 'json', child: Text('Export JSON')),
                              PopupMenuItem(value: 'txt', child: Text('Export TXT')),
                              PopupMenuItem(value: 'pdf', child: Text('Export PDF')),
                            ],
                          ),
                        ],
                      ),
                      const SizedBox(height: 12),
                       Wrap(
                         spacing: 8,
                         runSpacing: 8,
                         children: controller.mentionableUsers
                             .map((user) => ActionChip(
                                   label: Text('@$user'),
                                   onPressed: () => _insertMention(user),
                                 ))
                             .toList(growable: false),
                       ),
                      const SizedBox(height: 12),
                      Wrap(
                        spacing: 8,
                        runSpacing: 8,
                        children: _quickReplies
                            .map(
                              (item) => FilledButton.tonal(
                                onPressed: () async {
                                  try {
                                    await controller.sendCollaborationMessage(
                                      content: item['content']!,
                                      quickReply: item['quickReply']!,
                                      replyToId: _replyTarget?['id']?.toString() ?? '',
                                      type: 'quick_reply',
                                    );
                                    if (!mounted) return;
                                    setState(() => _replyTarget = null);
                                  } catch (error) {
                                    if (!mounted) return;
                                    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(error.toString())));
                                  }
                                },
                                child: Text(item['label']!),
                              ),
                            )
                            .toList(growable: false),
                      ),
                      const SizedBox(height: 16),
                      if (controller.chatLoading) const Center(child: CircularProgressIndicator()),
                      ...controller.collaborationMessages.map(
                        (message) => _MobileMessageBubble(
                          message: message,
                          isOwn: message['author']?.toString() == controller.chatUsername,
                          onReply: () => setState(() => _replyTarget = message),
                          onReact: (emoji) async {
                            try {
                              await controller.toggleReaction(message['id'].toString(), emoji);
                            } catch (error) {
                              if (!mounted) return;
                              ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(error.toString())));
                            }
                          },
                          onRecall: message['author']?.toString() == controller.chatUsername && message['status']?.toString() != 'recalled'
                              ? () async {
                                  try {
                                    await controller.recallMessage(message['id'].toString());
                                  } catch (error) {
                                    if (!mounted) return;
                                    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(error.toString())));
                                  }
                                }
                              : null,
                          onDelete: message['author']?.toString() == controller.chatUsername
                              ? () async {
                                  try {
                                    await controller.deleteMessage(message['id'].toString());
                                  } catch (error) {
                                    if (!mounted) return;
                                    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(error.toString())));
                                  }
                                }
                              : null,
                          reactionChoices: _reactions,
                        ),
                      ),
                      const SizedBox(height: 16),
                      if (_replyTarget != null)
                        Card(
                          child: ListTile(
                            title: Text('Replying to ${_replyTarget?['author'] ?? ''}'),
                            subtitle: Text(_replyTarget?['summary']?.toString() ?? ''),
                            trailing: IconButton(
                              onPressed: () => setState(() => _replyTarget = null),
                              icon: const Icon(Icons.close),
                            ),
                          ),
                        ),
                       TextField(
                          controller: _messageController,
                          minLines: 3,
                          maxLines: 6,
                          onTap: _syncMentionSuggestions,
                          decoration: const InputDecoration(
                           labelText: 'Message',
                           hintText: 'Type Markdown, mention teammates with @name, or send quick replies.',
                         ),
                       ),
                       if (_mentionSuggestions(controller).isNotEmpty) ...[
                          const SizedBox(height: 12),
                          Align(
                            alignment: Alignment.centerLeft,
                            child: Text('Mention recipients', style: Theme.of(context).textTheme.bodySmall),
                          ),
                         const SizedBox(height: 8),
                         Wrap(
                           spacing: 8,
                           runSpacing: 8,
                           children: _mentionSuggestions(controller)
                               .map(
                                 (user) => ActionChip(
                                   label: Text('@$user'),
                                   onPressed: () => _insertMention(user),
                                 ),
                               )
                               .toList(growable: false),
                         ),
                       ],
                       const SizedBox(height: 12),
                       Row(
                          children: [
                            FilledButton(
                              onPressed: () async {
                                try {
                                  await controller.markCollaborationRead();
                                  if (!mounted) return;
                                  ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Marked as read')));
                                } catch (error) {
                                  if (!mounted) return;
                                  ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(error.toString())));
                                }
                              },
                              child: const Text('Mark read'),
                            ),
                            const SizedBox(width: 12),
                            Expanded(
                              child: FilledButton(
                                onPressed: () async {
                                  if (_messageController.text.trim().isEmpty && _replyTarget == null) return;
                                  try {
                                    await controller.sendCollaborationMessage(
                                      content: _messageController.text.trim(),
                                      replyToId: _replyTarget?['id']?.toString() ?? '',
                                    );
                                    _messageController.clear();
                                    if (!mounted) {
                                      _replyTarget = null;
                                      _mentionQuery = null;
                                      return;
                                    }
                                    setState(() {
                                      _replyTarget = null;
                                      _mentionQuery = null;
                                    });
                                  } catch (error) {
                                    if (!mounted) return;
                                    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(error.toString())));
                                  }
                                },
                                child: const Text('Send'),
                              ),
                            ),
                          ],
                        ),
                    ],
                  ),
                ),
              ),
            ] else
              const Card(
                child: ListTile(
                  leading: Icon(Icons.info_outline),
                  title: Text('No linked collaboration room'),
                  subtitle: Text('This mobile installation is valid, but it is not bound to a collaboration session.'),
                ),
              ),
          ],
        ),
      ),
    );
  }

  List<String> _mentionSuggestions(KipupMobileController controller) {
    final query = _mentionQuery?.trim().toLowerCase();
    if (query == null) return const [];
    return controller.mentionableUsers
        .where((user) => user != controller.chatUsername && (query.isEmpty || user.toLowerCase().contains(query)))
        .toList(growable: false);
  }

  void _syncMentionSuggestions() {
    final selection = _messageController.selection;
    final cursor = selection.isValid && selection.baseOffset >= 0 ? selection.baseOffset : _messageController.text.length;
    final before = _messageController.text.substring(0, cursor);
    final match = _mentionDraftPattern.firstMatch(before);
    final nextQuery = match?.group(2);
    if (nextQuery == _mentionQuery) return;
    setState(() => _mentionQuery = nextQuery);
  }

  void _insertMention(String username) {
    final selection = _messageController.selection;
    final cursor = selection.isValid && selection.baseOffset >= 0 ? selection.baseOffset : _messageController.text.length;
    final before = _messageController.text.substring(0, cursor);
    final after = _messageController.text.substring(cursor);
    final match = _mentionDraftPattern.firstMatch(before);
    final start = match != null ? cursor - (match.group(2)?.length ?? 0) - 1 : cursor;
    final prefix = match != null ? _messageController.text.substring(0, start) : '${before}${before.endsWith(' ') || before.isEmpty ? '' : ' '}';
    final nextText = '$prefix@$username $after';
    final nextCursor = '$prefix@$username '.length;
    _messageController.value = TextEditingValue(
      text: nextText,
      selection: TextSelection.collapsed(offset: nextCursor),
    );
    setState(() => _mentionQuery = null);
  }
}

class _MobileMessageBubble extends StatelessWidget {
  const _MobileMessageBubble({
    required this.message,
    required this.isOwn,
    required this.onReply,
    required this.onReact,
    required this.reactionChoices,
    this.onRecall,
    this.onDelete,
  });

  final Map<String, dynamic> message;
  final bool isOwn;
  final VoidCallback onReply;
  final ValueChanged<String> onReact;
  final VoidCallback? onRecall;
  final VoidCallback? onDelete;
  final List<String> reactionChoices;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final reactions = ((message['reactions'] as List?) ?? const [])
        .map((item) => Map<String, dynamic>.from(item as Map))
        .toList(growable: false);
    return Align(
      alignment: isOwn ? Alignment.centerRight : Alignment.centerLeft,
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 540),
        child: Card(
          color: isOwn ? theme.colorScheme.primaryContainer : null,
          child: Padding(
            padding: const EdgeInsets.all(12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        message['author']?.toString() ?? 'Unknown',
                        style: theme.textTheme.titleSmall?.copyWith(fontWeight: FontWeight.w700),
                      ),
                    ),
                    Text(message['createdAt']?.toString().split('.').first.replaceFirst('T', ' ') ?? ''),
                  ],
                ),
                if (message['replyTo'] is Map)
                  Padding(
                    padding: const EdgeInsets.only(top: 8),
                    child: Container(
                      padding: const EdgeInsets.all(8),
                      decoration: BoxDecoration(
                        borderRadius: BorderRadius.circular(12),
                        color: theme.colorScheme.surfaceContainerHighest,
                      ),
                      child: Text(
                        '${(message['replyTo'] as Map)['author']}: ${(message['replyTo'] as Map)['summary']}',
                        style: theme.textTheme.bodySmall,
                      ),
                    ),
                  ),
                if ((message['quickReply']?.toString() ?? '').isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(top: 8),
                    child: Chip(label: Text(message['quickReply'].toString())),
                  ),
                Padding(
                  padding: const EdgeInsets.only(top: 8),
                  child: message['status']?.toString() == 'recalled'
                      ? Text('This message was recalled.', style: theme.textTheme.bodyMedium?.copyWith(fontStyle: FontStyle.italic))
                      : _MarkdownBubble(text: message['content']?.toString() ?? ''),
                ),
                if (reactions.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(top: 8),
                    child: Wrap(
                      spacing: 8,
                      runSpacing: 8,
                      children: reactions
                          .map(
                            (reaction) => ActionChip(
                              label: Text('${reaction['emoji']} ${((reaction['users'] as List?) ?? const []).length}'),
                              onPressed: () => onReact(reaction['emoji']?.toString() ?? ''),
                            ),
                          )
                          .toList(growable: false),
                    ),
                  ),
                const SizedBox(height: 8),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    TextButton(onPressed: onReply, child: const Text('Reply')),
                    ...reactionChoices.map((emoji) => TextButton(onPressed: () => onReact(emoji), child: Text(emoji))),
                    if (onRecall != null) TextButton(onPressed: onRecall, child: const Text('Recall')),
                    if (onDelete != null) TextButton(onPressed: onDelete, child: const Text('Delete')),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _MarkdownBubble extends StatelessWidget {
  const _MarkdownBubble({required this.text});

  final String text;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final blocks = text.split(RegExp(r'\n{2,}')).map((item) => item.trim()).where((item) => item.isNotEmpty).toList();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: blocks.map((block) {
        if (block.startsWith('>')) {
          final content = block.split('\n').map((line) => line.replaceFirst(RegExp(r'^>\s?'), '')).join('\n');
          return Container(
            width: double.infinity,
            margin: const EdgeInsets.only(top: 8),
            padding: const EdgeInsets.only(left: 12),
            decoration: BoxDecoration(
              border: Border(left: BorderSide(color: theme.colorScheme.outline)),
            ),
            child: Text.rich(_parseInline(content, theme.textTheme.bodyMedium ?? const TextStyle())),
          );
        }
        if (RegExp(r'^[-*]\s+', multiLine: true).hasMatch(block)) {
          final items = block.split('\n').where((item) => item.trim().isNotEmpty);
          return Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: items
                  .map((item) => Padding(
                        padding: const EdgeInsets.only(bottom: 4),
                        child: Text.rich(_parseInline('• ${item.replaceFirst(RegExp(r'^[-*]\s+'), '')}', theme.textTheme.bodyMedium ?? const TextStyle())),
                      ))
                  .toList(growable: false),
            ),
          );
        }
        final heading = RegExp(r'^(#{1,3})\s+(.+)$').firstMatch(block);
        if (heading != null) {
          return Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text.rich(
              _parseInline(
                heading.group(2) ?? '',
                (theme.textTheme.titleMedium ?? const TextStyle()).copyWith(fontWeight: FontWeight.w700),
              ),
            ),
          );
        }
        return Padding(
          padding: const EdgeInsets.only(top: 8),
          child: Text.rich(_parseInline(block, theme.textTheme.bodyMedium ?? const TextStyle())),
        );
      }).toList(growable: false),
    );
  }

  TextSpan _parseInline(String value, TextStyle baseStyle) {
    final pattern = RegExp(r'(\*\*[^*]+\*\*|`[^`]+`|~~[^~]+~~|\*[^*]+\*|@[A-Za-z0-9._-]{3,64})');
    final spans = <InlineSpan>[];
    var start = 0;
    for (final match in pattern.allMatches(value)) {
      if (match.start > start) {
        spans.add(TextSpan(text: value.substring(start, match.start), style: baseStyle));
      }
      final token = match.group(0) ?? '';
      if (token.startsWith('**') && token.endsWith('**')) {
        spans.add(TextSpan(text: token.substring(2, token.length - 2), style: baseStyle.copyWith(fontWeight: FontWeight.w700)));
      } else if (token.startsWith('*') && token.endsWith('*')) {
        spans.add(TextSpan(text: token.substring(1, token.length - 1), style: baseStyle.copyWith(fontStyle: FontStyle.italic)));
      } else if (token.startsWith('~~') && token.endsWith('~~')) {
        spans.add(TextSpan(text: token.substring(2, token.length - 2), style: baseStyle.copyWith(decoration: TextDecoration.lineThrough)));
      } else if (token.startsWith('`') && token.endsWith('`')) {
        spans.add(TextSpan(text: token.substring(1, token.length - 1), style: baseStyle.copyWith(fontFamily: 'monospace', backgroundColor: Colors.black12)));
      } else if (token.startsWith('@')) {
        spans.add(TextSpan(text: token, style: baseStyle.copyWith(fontWeight: FontWeight.w700, color: Colors.deepOrange)));
      } else {
        spans.add(TextSpan(text: token, style: baseStyle));
      }
      start = match.end;
    }
    if (start < value.length) {
      spans.add(TextSpan(text: value.substring(start), style: baseStyle));
    }
    return TextSpan(style: baseStyle, children: spans);
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
