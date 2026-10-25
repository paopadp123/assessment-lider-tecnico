import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import type { ReactNode } from "react";

import { Header } from "@/components/Header";
import { LiveEventsProvider } from "@/components/LiveEvents";
import { SESSION_COOKIE } from "@/lib/server/bff";

// Sin cookie de sesión no se muestra nada de operación. Si la cookie existe pero el token venció, la
// primera llamada al BFF responde 401, borra la cookie y la página vuelve al login.
export default async function OpsLayout({ children }: { children: ReactNode }) {
  if ((await cookies()).get(SESSION_COOKIE) === undefined) {
    redirect("/login");
  }
  return (
    <LiveEventsProvider>
      <Header />
      {children}
    </LiveEventsProvider>
  );
}
