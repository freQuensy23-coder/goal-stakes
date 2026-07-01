package com.goalstakes.mobile;

public final class Progress {
    public final Goal goal;
    public final String currentPeriod;
    public final boolean currentPeriodCompleted;
    public final int violationCount;

    public Progress(Goal goal, String currentPeriod, boolean currentPeriodCompleted, int violationCount) {
        this.goal = goal;
        this.currentPeriod = currentPeriod;
        this.currentPeriodCompleted = currentPeriodCompleted;
        this.violationCount = violationCount;
    }
}
