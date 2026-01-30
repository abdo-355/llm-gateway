export interface QuotaState {
  dailyRequests: number;
  dailyRequestsRemaining: number;
  rollingRpm: number;
  rollingTpm: number;
  lastReset: Date;
}

export class RollingWindow {
  private samples: number[];
  private index: number;

  constructor(size: number) {
    this.samples = new Array(size).fill(0);
    this.index = 0;
  }

  add(value: number): void {
    this.samples[this.index] = value;
    this.index = (this.index + 1) % this.samples.length;
  }

  getSum(): number {
    return this.samples.reduce((sum, val) => sum + val, 0);
  }

  getAverage(): number {
    return this.getSum() / this.samples.length;
  }
}

export class QuotaManager {
  private providerQuotas: Map<string, QuotaState>;
  private rpmWindows: Map<string, RollingWindow>;
  private tpmWindows: Map<string, RollingWindow>;

  constructor() {
    this.providerQuotas = new Map();
    this.rpmWindows = new Map();
    this.tpmWindows = new Map();
  }

  getQuotaState(providerId: string): QuotaState {
    let state = this.providerQuotas.get(providerId);
    if (!state) {
      state = {
        dailyRequests: 0,
        dailyRequestsRemaining: Infinity,
        rollingRpm: 0,
        rollingTpm: 0,
        lastReset: new Date(),
      };
      this.providerQuotas.set(providerId, state);
      this.rpmWindows.set(providerId, new RollingWindow(60)); // 60 seconds
      this.tpmWindows.set(providerId, new RollingWindow(60)); // 60 seconds for TPM sample
    }
    return state;
  }

  checkQuota(providerId: string, limits?: { daily_requests?: number; rpm?: number; tpm?: number }): { ok: boolean; remaining: number; headroomScore: number } {
    const state = this.getQuotaState(providerId);
    
    // Check daily limit
    if (limits?.daily_requests !== undefined) {
      if (state.dailyRequests >= limits.daily_requests) {
        return { ok: false, remaining: 0, headroomScore: 0 };
      }
    }

    // Check RPM
    if (limits?.rpm !== undefined) {
      if (state.rollingRpm >= limits.rpm) {
        return { ok: false, remaining: 0, headroomScore: 0 };
      }
    }

    // Check TPM
    if (limits?.tpm !== undefined) {
      if (state.rollingTpm >= limits.tpm) {
        return { ok: false, remaining: 0, headroomScore: 0 };
      }
    }

    // Calculate headroom score (0-1, where 1 is lots of headroom)
    let headroomScore = 1.0;
    
    if (limits?.daily_requests !== undefined && limits.daily_requests > 0) {
      const dailyHeadroom = 1 - (state.dailyRequests / limits.daily_requests);
      headroomScore = Math.min(headroomScore, dailyHeadroom);
    }

    if (limits?.rpm !== undefined && limits.rpm > 0) {
      const rpmHeadroom = 1 - (state.rollingRpm / limits.rpm);
      headroomScore = Math.min(headroomScore, rpmHeadroom);
    }

    // Calculate remaining requests (minimum across limits)
    let remaining = Infinity;
    if (limits?.daily_requests !== undefined) {
      remaining = Math.min(remaining, limits.daily_requests - state.dailyRequests);
    }
    if (limits?.rpm !== undefined) {
      remaining = Math.min(remaining, limits.rpm - state.rollingRpm);
    }

    return { ok: true, remaining, headroomScore };
  }

  recordRequest(providerId: string, tokensUsed?: number): void {
    const state = this.getQuotaState(providerId);
    state.dailyRequests++;

    const rpmWindow = this.rpmWindows.get(providerId);
    if (rpmWindow) {
      rpmWindow.add(1);
      state.rollingRpm = rpmWindow.getSum();
    }

    if (tokensUsed) {
      const tpmWindow = this.tpmWindows.get(providerId);
      if (tpmWindow) {
        tpmWindow.add(tokensUsed);
        state.rollingTpm = tpmWindow.getSum();
      }
    }
  }

  resetDaily(): void {
    for (const [providerId, state] of this.providerQuotas) {
      state.dailyRequests = 0;
      state.dailyRequestsRemaining = Infinity;
      state.lastReset = new Date();
    }
  }

  // Called periodically to update rolling windows
  tick(): void {
    for (const [providerId, rpmWindow] of this.rpmWindows) {
      rpmWindow.add(0); // Shift window with zero
      const state = this.providerQuotas.get(providerId);
      if (state) {
        state.rollingRpm = rpmWindow.getSum();
      }
    }

    for (const [providerId, tpmWindow] of this.tpmWindows) {
      tpmWindow.add(0); // Shift window with zero
      const state = this.providerQuotas.get(providerId);
      if (state) {
        state.rollingTpm = tpmWindow.getSum();
      }
    }
  }

  getState(): Map<string, { remaining: number; headroomScore: number }> {
    const result = new Map<string, { remaining: number; headroomScore: number }>();
    for (const [providerId, state] of this.providerQuotas) {
      result.set(providerId, {
        remaining: state.dailyRequestsRemaining,
        headroomScore: state.dailyRequestsRemaining / (state.dailyRequests + state.dailyRequestsRemaining || 1),
      });
    }
    return result;
  }
}
