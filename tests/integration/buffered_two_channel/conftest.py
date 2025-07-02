"""Configurations for the experimental setup of a two-channel device."""

import pytest


@pytest.fixture
def expectedInstruments():
    """Returns a list of instruments that should be running."""
    return [
        "BufferedMultiChannelDACLeader",
        "BufferedMultiChannelDACFollower",
        "MultiChannelAmnmeter",
    ]


@pytest.fixture
def deviceConfig():
    """Returns the device configuration for testing."""
    return {
        "ScreeningGates": "S1;S2;S3",
        "PlungerGates": "P1;P2;P3;P4",
        "Ohmics": "O1;O2;O3;O4",
        "BarrierGates": "B1;B2;B3;B4;B5;B6",
        "ReservoirGates": "R1;R2;R3;R4",
        "num-unique-channels": 2,
        "groups": {
            "group1": {
                "Name": "I_O1",
                "NumDots": 3,
                "ScreeningGates": "S1;S2",
                "ReservoirGates": "R1;R2",
                "PlungerGates": "P1;P2;P3",
                "BarrierGates": "B1;B2;B3:B4",
                "Order": "O1;R1;B1;P1;B2;P2;B3;P3;B4;R2;O2",
            },
            "group2": {
                "Name": "I_O3",
                "NumDots": 1,
                "ScreeningGates": "S2;S3",
                "ReservoirGates": "R3;R4",
                "PlungerGates": "P4",
                "BarrierGates": "B5;B6",
                "Order": "O3;R3;B5;P4;B6;R4;O4",
            },
        },
        "wiringDC": {
            "S1": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "S2": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "S3": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "P1": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "P2": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "P3": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "P4": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "O1": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "O2": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "O3": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "O4": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "R1": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "R2": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "R3": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "R4": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "B1": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "B2": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "B3": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "B4": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "B5": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "B6": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
        },
    }


@pytest.fixture
def wiremap():
    """Returns a wiremap for testing."""
    return {
        "BufferedMultiChannelDACLeader.0": "S1",
        "BufferedMultiChannelDACLeader.1": "S2",
        "BufferedMultiChannelDACLeader.2": "S3",
        "BufferedMultiChannelDACLeader.3": "B1",
        "BufferedMultiChannelDACLeader.4": "B2",
        "BufferedMultiChannelDACLeader.5": "B3",
        "BufferedMultiChannelDACLeader.6": "B4",
        "BufferedMultiChannelDACLeader.7": "B5",
        "BufferedMultiChannelDACLeader.8": "B6",
        "BufferedMultiChannelDACLeader.9": "P1",
        "BufferedMultiChannelDACFollower.0": "P2",
        "BufferedMultiChannelDACFollower.1": "P3",
        "BufferedMultiChannelDACFollower.2": "P4",
        "BufferedMultiChannelDACFollower.3": "R1",
        "BufferedMultiChannelDACFollower.4": "R2",
        "BufferedMultiChannelDACFollower.5": "R3",
        "BufferedMultiChannelDACFollower.6": "R4",
        "MultiChannelAmnmeter.1": "O2",
        "MultiChannelAmnmeter.2": "O4",
    }
