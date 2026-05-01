# Kipup Mobile

Minimal Flutter client scaffold for the mobile distribution flow.

## Current scope

- First-launch activation with a generated download token
- Startup validation against `/api/v1/mobile/installations/validate`
- Offline grace handling
- Automatic local reset when the backend reports expiry or revocation

## Local setup

```bash
cd /home/runner/work/kipup/kipup/mobile_app
flutter pub get
flutter run --dart-define KIPUP_API_BASE_URL=http://localhost:8080/api/v1
```

Open the generated mobile download page in Kipup, copy the activation code, and paste it into the app on first launch.
