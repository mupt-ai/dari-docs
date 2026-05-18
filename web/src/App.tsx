import { Navigate, Route, Routes } from "react-router-dom";

import { useAuthState } from "@/lib/auth";
import AppLayout from "@/routes/AppLayout";
import AuthCallback from "@/routes/AuthCallback";
import Billing from "@/routes/Billing";
import Login from "@/routes/Login";
import RunDetail from "@/routes/RunDetail";
import Runs from "@/routes/Runs";
import Settings from "@/routes/Settings";
import Tokens from "@/routes/Tokens";

export default function App() {
  const auth = useAuthState();

  if (auth.status === "loading") {
    return (
      <div className="flex h-screen items-center justify-center text-muted-foreground">
        loading...
      </div>
    );
  }

  return (
    <Routes>
      <Route path="/auth/callback" element={<AuthCallback />} />
      {auth.status === "signed_out" ? (
        <>
          <Route path="/login" element={<Login />} />
          <Route path="*" element={<Navigate to="/login" replace />} />
        </>
      ) : (
        <Route element={<AppLayout profile={auth.profile} />}>
          <Route index element={<Navigate to="/runs" replace />} />
          <Route path="/runs" element={<Runs />} />
          <Route path="/runs/:runId" element={<RunDetail />} />
          <Route path="/billing" element={<Billing />} />
          <Route path="/tokens" element={<Tokens />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/runs" replace />} />
        </Route>
      )}
    </Routes>
  );
}
