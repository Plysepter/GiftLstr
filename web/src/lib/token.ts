// Holds the current Clerk session token getter so that api.ts does not
// depend on auth.ts (which itself uses api.ts).
let tokenGetter: (() => Promise<string | null>) | null = null;

export function setTokenGetter(getter: () => Promise<string | null>): void {
  tokenGetter = getter;
}

export async function currentToken(): Promise<string | null> {
  if (!tokenGetter) return null;
  return tokenGetter();
}
