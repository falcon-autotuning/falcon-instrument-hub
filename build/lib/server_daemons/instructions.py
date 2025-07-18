"""The instuctions for a measurement interpretet."""

from typing import TYPE_CHECKING

from .dependancies import SUPPORTED_PROPERTIES
from .typing import (
    Knob,
)

if TYPE_CHECKING:
    from .typing import (
        Getters,
        InstrumentPort,
        Meter,
        PropertyName,
        PropertyValue,
        Requirements,
        Setters,
    )


class Instruction:
    """A class that holds a single instruction for a measurement.

    A measurement is a thing that needs to be executed on instrument daemons running on the runtime instrument server.
    It is broken down into a atomic instructions that can be executed on order on the instruments to satify the measurement.
    This consists of three main parts:
        requirements: A dictionary of connections and their properties to be set.
        setters: A list of connections to set for the measurement.
        getters: A list of connections to get data from.
    Getters are no required, but setters are, otherwise there is no measurement to perform.
    """

    _setters: "Setters"
    _getters: "Getters"
    _requirements: "Requirements"
    _buffered: bool

    def __init__(
        self,
        setters: "Setters | None" = None,
        requirements: "Requirements | None" = None,
        getters: "Getters | None" = None,
        buffered: bool = False,
    ):
        """Initialize the instruction."""
        if getters is None:
            self._getters = []
        else:
            self._getters = getters
        if setters is None:
            self._setters = []
        else:
            self._setters = setters
        if requirements is None:
            self._requirements = {}
        else:
            self._requirements = requirements
        self._buffered = buffered

    @property
    def getters(self) -> "Getters":
        """The getters for the instruction."""
        return self._getters

    def add_getter(
        self,
        instrument: "Meter",
    ) -> None:
        """Add a getter to the instruction."""
        self._getters = [*self._getters, instrument]

    @property
    def setters(self) -> "Setters":
        """The setters for the instruction."""
        return self._setters

    def add_setter(
        self,
        instrument: "Knob",
    ) -> None:
        """Add a setter to the instruction."""
        self._setters = [*self._setters, instrument]

    @property
    def requirements(self) -> "Requirements":
        """The requirements for the instruction."""
        return self._requirements

    def add_requirement(
        self,
        instrument: "InstrumentPort",
        properties: dict["PropertyName", "PropertyValue"],
    ) -> None:
        """Add a setter to the instruction."""
        self._requirements[instrument] = properties

    @property
    def buffered(self) -> bool:
        """If the instruction is buffered."""
        return self._buffered

    def __repr__(self) -> str:
        """Returns a string representation of the instruction."""
        return (
            f"Instruction(setters={self._setters}, getters={self._getters}, "
            f"buffered={self._buffered})"
        )

    def retrieve_voltage_states(self) -> dict["Knob", float]:
        """Unpacks the requirements to get any setters that are setting voltage states."""
        map: dict[Knob, float] = {}
        for port, requirements in self.requirements.items():
            if port not in self.setters:
                continue
            if SUPPORTED_PROPERTIES.VOLTAGE_STATE in requirements:
                v_state = requirements[SUPPORTED_PROPERTIES.VOLTAGE_STATE]
                assert isinstance(v_state, float)
                assert isinstance(port, Knob), (
                    "The port used in the sweep was not a knob"
                )
                map[port] = v_state

        return map

    def contains_buffered_measurement(self) -> int:
        """Flag that indicates a buffered measurement is present."""
        for port, requirements in self.requirements.items():
            if port not in self.setters:
                continue
            if SUPPORTED_PROPERTIES.STAIRCASE in requirements:
                staircase = requirements[SUPPORTED_PROPERTIES.STAIRCASE]
                assert isinstance(staircase, tuple), (
                    "STAIRCASE must be a tuple of numbers."
                )
                assert isinstance(staircase[1], int), (
                    "STAIRCASE[1] (num_steps)  must be an integer."
                )
                return staircase[1]
        return 0

    def seperate_buffered_requirements(
        self,
    ) -> list[tuple["InstrumentPort", "PropertyValue"]]:
        """Seperates the buffered measurements from the rest."""
        outs: list[tuple[InstrumentPort, PropertyValue]] = []
        for port, requirements in self.requirements.items():
            if port not in self.setters:
                continue
            if SUPPORTED_PROPERTIES.STAIRCASE in requirements:
                staircase = requirements[SUPPORTED_PROPERTIES.STAIRCASE]
                outs.append((port, staircase))

        return outs

    def retrieve_buffered_voltage_states(
        self, num_steps: int
    ) -> list[dict["InstrumentPort", float]]:
        """Unpacks the requirements to get any setters that are setting buffered voltage states."""
        maps: list[dict[InstrumentPort, float]] = []
        for i in range(num_steps):
            for name, staircase in self.seperate_buffered_requirements():
                map: dict[InstrumentPort, float] = {}
                assert isinstance(staircase, tuple), (
                    "STAIRCASE must be a tuple of numbers."
                )
                v_stop = staircase[4]
                assert isinstance(v_stop, float), (
                    "STAIRCASE[4] (v_stop) must be a float."
                )
                v_start = staircase[3]
                assert isinstance(v_start, float), (
                    "STAIRCASE[3] (v_start) must be a float."
                )
                map[name] = ((v_stop - v_start) * i / (num_steps - 1)) + v_start
                maps.append(map)
        return maps


class MeasurementInstructions:
    """A class that holds the instructions for a measurement.

    Typically a measurement consists of a list of steps, each step containing directions on what instruments to use, what to set and what to collect, if anything.
    This class is used to store all of the steps in the form of a measurement.
    """

    _instructions: list[Instruction]

    def __init__(self, instructions: list[Instruction] = []):
        """Starts up the insructions object."""
        self._instructions = instructions

    @property
    def instructions(self) -> list[Instruction]:
        """Returns the instructions for the measurement."""
        return self._instructions

    def __iter__(self):
        """Iterate over the instructions."""
        return iter(self._instructions)

    def __len__(self):
        """Returns the length of the instructions."""
        return len(self._instructions)

    def __getitem__(self, index: int) -> Instruction:
        """Gets a specific instruction."""
        return self._instructions[index]

    def __setitem__(self, index: int, value: Instruction) -> None:
        """Sets a specific instruction."""
        self._instructions[index] = value

    def __delitem__(self, index: int) -> None:
        """Deletes a specific instruction."""
        self._instructions.pop(index)

    def __contains__(self, item: Instruction) -> bool:
        """Checks if an instruction is in the list."""
        return item in self._instructions

    def __repr__(self) -> str:
        """Returns a string representation of the instructions."""
        return f"MeasurementInstructions({self._instructions})"

    def __str__(self) -> str:
        """Returns a string representation of the instructions."""
        return f"MeasurementInstructions({self._instructions})"
