/// Accept only the two application routes we publish. Unrelated URLs and
/// malformed/decorated codes never start a join or trigger authentication.
String? roomCodeFromRoute(String? route) {
  if (route == null) return null;
  final uri = Uri.tryParse(route);
  if (uri == null ||
      uri.hasQuery ||
      uri.hasFragment ||
      uri.userInfo.isNotEmpty) {
    return null;
  }
  final parts = uri.pathSegments;
  String? code;
  if (uri.scheme.isEmpty &&
      !uri.hasAuthority &&
      uri.path.startsWith('/') &&
      parts.length == 2 &&
      parts.first == 'join') {
    code = parts.last;
  } else if (uri.scheme == 'knowoff' &&
      uri.host == 'join' &&
      !uri.hasPort &&
      parts.length == 1) {
    code = parts.single;
  }
  return code != null && RegExp(r'^[A-Za-z0-9]{6}$').hasMatch(code)
      ? code.toUpperCase()
      : null;
}

/// The web link keeps the current app's deployment path and uses Flutter's
/// default hash routing; native clients publish their registered scheme.
String roomShareLink(String code, {required Uri base, required bool web}) {
  final normalized = roomCodeFromRoute('/join/$code');
  if (normalized == null) {
    throw ArgumentError.value(code, 'code', 'Invalid room code');
  }
  if (!web) return 'knowoff://join/$normalized';
  return Uri(
          scheme: base.scheme,
          host: base.host,
          port: base.hasPort ? base.port : null,
          path: base.path,
          fragment: '/join/$normalized')
      .toString();
}
