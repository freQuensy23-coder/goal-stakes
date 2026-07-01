package com.goalstakes.mobile;

public final class Goal {
    public final String id;
    public final String title;
    public final String type;
    public final String cadence;
    public final String stakeAmount;
    public final String tokenSymbol;
    public final String chain;
    public final String startsAt;
    public final String endsAt;

    public Goal(String id, String title, String type, String cadence, String stakeAmount, String tokenSymbol, String chain) {
        this(id, title, type, cadence, stakeAmount, tokenSymbol, chain, "", "");
    }

    public Goal(String id, String title, String type, String cadence, String stakeAmount, String tokenSymbol, String chain, String startsAt, String endsAt) {
        this.id = id;
        this.title = title;
        this.type = type;
        this.cadence = cadence;
        this.stakeAmount = stakeAmount;
        this.tokenSymbol = tokenSymbol;
        this.chain = chain;
        this.startsAt = startsAt == null ? "" : startsAt;
        this.endsAt = endsAt == null ? "" : endsAt;
    }
}
