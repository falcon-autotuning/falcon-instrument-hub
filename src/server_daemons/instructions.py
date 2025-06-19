"""The instuctions for a measurement interpretet."""

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .typing import (
        Getters,
        InstrumentPort,
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
        setters: "Setters" = [],
        requirements: "Requirements" = {},
        getters: "Getters" = [],
        buffered: bool = False,
    ):
        """Initialize the instruction."""
        self._getters = getters
        self._setters = setters
        self._requirements = requirements
        self._buffered = buffered

    @property
    def getters(self) -> "Getters":
        """The getters for the instruction."""
        return self._getters

    def add_getter(
        self,
        instrument: "InstrumentPort",
    ) -> None:
        """Add a getter to the instruction."""
        self._getters.append(instrument)

    @property
    def setters(self) -> "Setters":
        """The setters for the instruction."""
        return self._setters

    def add_setter(
        self,
        instrument: "InstrumentPort",
    ) -> None:
        """Add a setter to the instruction."""
        self._setters.append(instrument)

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
