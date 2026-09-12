import '../../data/auth_service.dart';
import '../../data/api_client.dart';
import '../../data/native_purchase_bridge.dart';
import '../../data/purchase_controller.dart';
import 'client_config.dart';

/// Global application configuration and services.
class AppConfig {
  AppConfig._(this.clientConfig, [dynamic authService])
    : authService = authService ?? AuthService(baseUrl: clientConfig.serverUrl);

  static AppConfig? _instance;

  /// Initializes the global config. If [config] is omitted it is loaded.
  static Future<AppConfig> initialize([
    ClientConfig? config,
    dynamic authService,
  ]) async {
    final cfg = config ?? await ClientConfig.load();
    _instance = AppConfig._(cfg, authService);
    // Authentication is refreshed lazily by actions that need a token. Do
    // not make mounting the public shell depend on backend availability.
    return _instance!;
  }

  static AppConfig get instance =>
      _instance ?? AppConfig._(ClientConfig.defaultConfig());

  final ClientConfig clientConfig;
  final AuthService authService;
  late final PurchaseController purchases = PurchaseController(
    auth: authService,
    api: ApiClient(baseUrl: serverUrl, auth: authService),
    bridge: NativePurchaseBridge(),
  );

  String get serverUrl => clientConfig.serverUrl;
  String get websocketUrl => clientConfig.websocketUrl;
}
